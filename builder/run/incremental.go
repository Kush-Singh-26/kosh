package run

import (
	"bytes"
	"fmt"
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
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

// invalidateForTemplate determines which posts to invalidate based on changed template
// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	tp := filepath.ToSlash(templatePath)
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.TemplateDir)) {
		relTmpl, _ := filepath.Rel(b.cfg.TemplateDir, tp)
		relTmpl = filepath.ToSlash(relTmpl)

		if relTmpl == "layout.html" {
			return nil // Layout changes affect everything
		}

		if b.cacheManager != nil {
			ids, err := b.cacheManager.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				// Batch fetch all posts in a single transaction (avoids N+1 query)
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
		// Sync VFS changes to disk (differential)
		if err := utils.SyncVFS(b.DestFs, "public", b.rnd.GetRenderedFiles()); err != nil {
			fmt.Printf("❌ Sync failed: %v\n", err)
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
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		b.Build()
		return
	}

	context := parser.NewContext()
	reader := text.NewReader(source)
	b.md.Parser().Parse(reader, parser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	relPath, _ := filepath.Rel("content", path)

	// Check Cache
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
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		return
	}

	info, _ := b.SourceFs.Stat(path)
	relPath, _ := filepath.Rel("content", path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	destPath := filepath.Join("public", htmlRelPath)
	fullLink := cfg.BaseURL + "/" + htmlRelPath

	context := parser.NewContext()
	reader := text.NewReader(source)
	docNode := b.md.Parser().Parse(reader, parser.WithContext(context))

	var buf bytes.Buffer
	// Handle diagrams (simplified for partial build)
	_ = b.md.Renderer().Render(&buf, source, docNode)
	htmlContent := buf.String()

	if pairs := mdParser.GetD2SVGPairSlice(context); pairs != nil {
		htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
	}

	// Use DiagramAdapter map equivalent
	var diagramCache map[string]string
	if b.diagramAdapter != nil {
		diagramCache = b.diagramAdapter.AsMap()
	}

	hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")
	if hasMath {
		htmlContent = mdParser.RenderMathForHTML(htmlContent, b.native, diagramCache, &b.mu)
	}

	if cfg.CompressImages {
		htmlContent = utils.ReplaceToWebP(htmlContent)
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
	hasD2 := mdParser.HasD2(context)

	post := models.PostMetadata{
		Title:       utils.GetString(metaData, "title"),
		Link:        fullLink,
		Description: utils.GetString(metaData, "description"),
		Tags:        utils.GetSlice(metaData, "tags"),
		ReadingTime: readTime,
		Pinned:      isPinned,
		Draft:       isDraft,
		DateObj:     dateObj,
		HasMath:     hasMath,
		HasMermaid:  hasD2,
	}

	// Convert TOC for cache (uses unified models.TOCEntry type)
	cacheTOC := make([]models.TOCEntry, len(toc))
	for i, t := range toc {
		cacheTOC[i] = models.TOCEntry{
			ID:    t.ID,
			Text:  t.Text,
			Level: t.Level,
		}
	}

	frontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	if isDraft && !cfg.IncludeDrafts {
		return
	}

	// Update Cache in BoltDB
	if b.cacheManager != nil {
		htmlHash, _ := b.cacheManager.StoreHTML([]byte(htmlContent))
		postID := cache.GeneratePostID("", relPath)

		newMeta := &cache.PostMeta{
			PostID:      postID,
			Path:        relPath,
			ModTime:     info.ModTime().Unix(),
			ContentHash: frontmatterHash,
			HTMLHash:    htmlHash,
			Title:       post.Title,
			Date:        post.DateObj,
			Tags:        post.Tags,
			ReadingTime: post.ReadingTime,
			Description: post.Description,
			Link:        post.Link,
			Pinned:      post.Pinned,
			Draft:       post.Draft,
			HasMath:     post.HasMath,
			HasMermaid:  post.HasMermaid,
			Meta:        metaData,
			TOC:         cacheTOC,
		}

		// Search Record update (needed for partial build?)
		searchRecord := models.PostRecord{
			Title:       post.Title,
			Link:        htmlRelPath,
			Description: post.Description,
			Tags:        post.Tags,
			Content:     plainText,
		}
		fullText := strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content)
		words := search.Tokenize(fullText)
		docLen := len(words)
		wordFreqs := make(map[string]int)
		for _, w := range words {
			if len(w) >= 2 {
				wordFreqs[w]++
			}
		}

		newSearch := &cache.SearchRecord{
			Title:    post.Title,
			Tokens:   search.Tokenize(post.Description),
			BM25Data: wordFreqs,
			DocLen:   docLen,
		}

		newDep := &cache.Dependencies{Tags: post.Tags}

		// Single Commit
		_ = b.cacheManager.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*cache.Dependencies{postID: newDep})
	}

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
		Title:        post.Title,
		Description:  post.Description,
		Content:      template.HTML(htmlContent),
		Meta:         metaData,
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
		TabTitle:     post.Title + " | " + cfg.Title,
		Permalink:    fullLink,
		Image:        imagePath,
		HasMath:      hasMath,
		HasMermaid:   hasD2,
		TOC:          toc,
		Config:       cfg,
	})
}
