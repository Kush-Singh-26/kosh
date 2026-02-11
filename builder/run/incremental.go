package run

import (
	"bytes"
	"html/template"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/cache"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/utils"
)

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	tp := filepath.ToSlash(templatePath)
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.TemplateDir)) {
		relTmpl, _ := utils.SafeRel(b.cfg.TemplateDir, tp)
		relTmpl = filepath.ToSlash(relTmpl)

		if relTmpl == "layout.html" {
			return nil // Layout changes affect everything
		}

		if b.cacheManager != nil {
			ids, err := b.cacheManager.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				posts, err := b.cacheManager.GetPostsByIDs(ids)
				if err == nil && len(posts) > 0 {
					paths := make([]string, 0, len(posts))
					for _, post := range posts {
						paths = append(paths, post.Path)
					}
					return paths
				}
			}
		}
		return []string{}
	}
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.StaticDir)) {
		return nil
	}

	switch tp {
	case "kosh.yaml":
		return nil
	case "builder/generators/pwa.go":
		return []string{}
	default:
		return nil
	}
}

// BuildChanged rebuilds only the changed file (for watch mode)
func (b *Builder) BuildChanged(changedPath string) {
	if strings.HasSuffix(changedPath, ".md") && strings.HasPrefix(changedPath, "content") {
		b.buildSinglePost(changedPath)
		if err := utils.SyncVFS(b.DestFs, "public", b.rnd.GetRenderedFiles()); err != nil {
			b.logger.Error("Sync failed", "error", err)
		}
		b.rnd.ClearRenderedFiles()
		return
	}

	b.Build()
	b.SaveCaches()
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Builder) buildSinglePost(path string) {
	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		b.logger.Error("Error reading file", "path", path, "error", err)
		b.Build()
		return
	}

	context := parser.NewContext()
	context.Set(mdParser.ContextKeyFilePath, path)
	reader := text.NewReader(source)
	b.md.Parser().Parse(reader, parser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	relPath, _ := utils.SafeRel("content", path)

	var exists bool
	var cachedHash string

	if b.cacheManager != nil {
		if meta, err := b.cacheManager.GetPostByPath(relPath); err == nil && meta != nil {
			exists = true
			cachedHash = meta.ContentHash
		}
	}

	if exists && cachedHash == newFrontmatterHash {
		b.buildContentOnly(path)
		b.SaveCaches()
	} else {
		// Full rebuild handles cache update
		b.Build()
		b.SaveCaches()
	}
}

// buildContentOnly rebuilds just a single post's HTML without regenerating global pages
func (b *Builder) buildContentOnly(path string) {
	cfg := b.cfg

	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		b.logger.Error("Error reading file", "path", path, "error", err)
		return
	}

	info, _ := b.SourceFs.Stat(path)
	version, relPath := utils.GetVersionFromPath(path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

	// Clean HTML relative path for versioned posts
	cleanHtmlRelPath := htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
	}

	var destPath string
	if version != "" {
		destPath = filepath.Join("public", version, cleanHtmlRelPath)
	} else {
		destPath = filepath.Join("public", htmlRelPath)
	}
	fullLink := utils.BuildPostLink(cfg.BaseURL, version, cleanHtmlRelPath)

	context := parser.NewContext()
	context.Set(mdParser.ContextKeyFilePath, path)
	reader := text.NewReader(source)
	docNode := b.md.Parser().Parse(reader, parser.WithContext(context))

	var buf bytes.Buffer
	_ = b.md.Renderer().Render(&buf, source, docNode)
	htmlContent := buf.String()

	if pairs := mdParser.GetD2SVGPairSlice(context); pairs != nil {
		htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
	}

	var diagramCache map[string]string
	if b.diagramAdapter != nil {
		diagramCache = b.diagramAdapter.AsMap()
	}

	if strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(") {
		htmlContent = mdParser.RenderMathForHTML(htmlContent, b.native, diagramCache, &b.mu)
	}
	if cfg.CompressImages {
		htmlContent = utils.ReplaceToWebP(htmlContent)
	}

	// Copy raw markdown to output for "View Source" feature
	if cfg.Features.RawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		_ = b.DestFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
		_ = afero.WriteFile(b.DestFs, mdDestPath, source, 0644)
	}

	metaData := meta.Get(context)
	plainText := mdParser.ExtractPlainText(docNode, source)
	wordCount := len(strings.Fields(string(source)))
	readTime := int(math.Ceil(float64(wordCount) / 120.0))
	isPinned, _ := metaData["pinned"].(bool)
	dateStr := utils.GetString(metaData, "date")
	dateObj, _ := time.Parse("2006-01-02", dateStr)
	isDraft := utils.GetBool(metaData, "draft")

	toc := mdParser.GetTOC(context)

	post := models.PostMetadata{
		Title:       utils.GetString(metaData, "title"),
		Link:        fullLink,
		Description: utils.GetString(metaData, "description"),
		Tags:        utils.GetSlice(metaData, "tags"),
		ReadingTime: readTime,
		Pinned:      isPinned,
		Draft:       isDraft,
		DateObj:     dateObj,
		Version:     version,
	}

	// Fetch other posts in the same version to build sidebar and neighbors
	var versionPosts []models.PostMetadata
	if b.cacheManager != nil {
		ids, _ := b.cacheManager.ListAllPosts()
		cachedPosts, _ := b.cacheManager.GetPostsByIDs(ids)
		for _, cp := range cachedPosts {
			if cp.Version == version {
				versionPosts = append(versionPosts, models.PostMetadata{
					Title: cp.Title, Link: cp.Link, Weight: cp.Weight, Version: cp.Version,
					DateObj: cp.Date,
				})
			}
		}
	}
	// Sync current post in the list
	found := false
	for i, p := range versionPosts {
		if p.Link == post.Link {
			versionPosts[i] = post
			found = true
			break
		}
	}
	if !found {
		versionPosts = append(versionPosts, post)
	}

	// Calculate Neighbors & Sidebar
	utils.SortPosts(versionPosts)
	prev, next := utils.FindPrevNext(post, versionPosts)
	siteTree := utils.BuildSiteTree(versionPosts)

	// Update Cache in BoltDB
	if b.cacheManager != nil {
		htmlHash, _ := b.cacheManager.StoreHTML([]byte(htmlContent))
		postID := cache.GeneratePostID("", relPath)
		cacheTOC := make([]models.TOCEntry, len(toc))
		for i, t := range toc {
			cacheTOC[i] = models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level}
		}

		newMeta := &cache.PostMeta{
			PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
			ContentHash: cache.HashString(string(source)), HTMLHash: htmlHash,
			Title: post.Title, Date: post.DateObj, Tags: post.Tags,
			ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight,
			Draft: post.Draft, Meta: metaData, TOC: cacheTOC, Version: version,
		}

		newSearch := &cache.SearchRecord{
			Title: post.Title, BM25Data: make(map[string]int), DocLen: wordCount, Content: plainText,
		}
		newDep := &cache.Dependencies{Tags: post.Tags}
		_ = b.cacheManager.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*cache.Dependencies{postID: newDep})
	}

	// Determine Card Image
	cardRelPath := strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	imagePath := cfg.BaseURL + "/static/images/cards/" + cardRelPath
	if img, ok := metaData["image"].(string); ok {
		if cfg.CompressImages && !strings.HasPrefix(img, "http") {
			ext := filepath.Ext(img)
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
				img = img[:len(img)-len(ext)] + ".webp"
			}
		}
		imagePath = cfg.BaseURL + img
	}

	b.rnd.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: metaData, BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion,
		TabTitle: post.Title + " | " + cfg.Title, Permalink: post.Link, Image: imagePath,
		TOC: toc, Config: cfg, SiteTree: siteTree,
		CurrentVersion: version, IsOutdated: version != "",
		Versions: cfg.GetVersionsMetadata(version),
		PrevPage: prev, NextPage: next,
	})
}
