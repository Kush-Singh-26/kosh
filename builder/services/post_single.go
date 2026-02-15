package services

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	meta "github.com/yuin/goldmark-meta"
	gParser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (s *postServiceImpl) ProcessSingle(ctx context.Context, path string) error {
	info, err := s.sourceFs.Stat(path)
	if err != nil {
		s.logger.Error("Error stating file", "path", path, "error", err)
		return err
	}

	// Check file size before loading into memory
	if info.Size() > utils.MaxFileSize {
		s.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", utils.MaxFileSize)
		return fmt.Errorf("file size %d exceeds limit %d", info.Size(), utils.MaxFileSize)
	}

	source, err := afero.ReadFile(s.sourceFs, path)
	if err != nil {
		s.logger.Error("Error reading file", "path", path, "error", err)
		return err
	}

	version, relPath := utils.GetVersionFromPath(path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

	cleanHtmlRelPath := htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
	}

	var destPath string
	if version != "" {
		destPath = filepath.Join(s.cfg.OutputDir, version, cleanHtmlRelPath)
	} else {
		destPath = filepath.Join(s.cfg.OutputDir, htmlRelPath)
	}
	fullLink := utils.BuildURL(s.cfg.BaseURL, version, cleanHtmlRelPath)

	context := gParser.NewContext()
	context.Set(mdParser.ContextKeyFilePath, path)
	reader := text.NewReader(source)
	docNode := s.md.Parser().Parse(reader, gParser.WithContext(context))

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	if err := s.md.Renderer().Render(buf, source, docNode); err != nil {
		s.logger.Error("Failed to render markdown", "path", path, "error", err)
		return err
	}
	htmlContent := buf.String()

	if pairs := mdParser.GetD2SVGPairSlice(context); pairs != nil {
		htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
	}

	var diagramCache map[string]string
	if s.diagramAdapter != nil {
		diagramCache = s.diagramAdapter.AsMap()
	}

	ssrHashes := mdParser.GetSSRHashes(context)

	if bytes.Contains(source, []byte("$")) || bytes.Contains(source, []byte("\\(")) {
		var mathHashes []string
		htmlContent, mathHashes = mdParser.RenderMathForHTML(htmlContent, s.nativeRenderer, diagramCache, &s.mu)
		ssrHashes = append(ssrHashes, mathHashes...)
	}
	if s.cfg.CompressImages {
		htmlContent = utils.ReplaceToWebP(htmlContent)
	}

	if s.cfg.Features.RawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		_ = s.destFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
		_ = afero.WriteFile(s.destFs, mdDestPath, source, 0644)
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

	var versionPosts []models.PostMetadata
	if s.cache != nil {
		// Use optimized version query instead of loading all posts
		versionMetas, err := s.cache.GetPostsMetadataByVersion(version)
		if err == nil {
			versionPosts = make([]models.PostMetadata, len(versionMetas))
			for i, m := range versionMetas {
				versionPosts[i] = models.PostMetadata{
					Title:   m.Title,
					Link:    m.Link,
					Weight:  m.Weight,
					Version: m.Version,
					DateObj: m.Date,
				}
			}
		}
	}

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

	utils.SortPosts(versionPosts)
	prev, next := utils.FindPrevNext(post, versionPosts)
	siteTree := utils.BuildSiteTree(versionPosts, post.Link)

	if s.cache != nil {
		htmlHash, _ := s.cache.StoreHTML([]byte(htmlContent))

		postID := cache.GeneratePostID("", relPath)
		cacheTOC := make([]models.TOCEntry, len(toc))
		for i, t := range toc {
			cacheTOC[i] = models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level}
		}

		frontmatterHash, _ := utils.GetFrontmatterHash(metaData)
		bodyHash := utils.GetBodyHash(source)

		newMeta := &cache.PostMeta{
			PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
			ContentHash: frontmatterHash, BodyHash: bodyHash, HTMLHash: htmlHash,
			Title: post.Title, Date: post.DateObj, Tags: post.Tags,
			ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight,
			Draft: post.Draft, Meta: metaData, TOC: cacheTOC, Version: version,
			SSRInputHashes: ssrHashes,
		}

		normalizedTags := make([]string, len(post.Tags))
		for i, t := range post.Tags {
			normalizedTags[i] = strings.ToLower(t)
		}

		newSearch := &cache.SearchRecord{
			Title: post.Title, NormalizedTitle: strings.ToLower(post.Title),
			BM25Data: make(map[string]int), DocLen: wordCount, Content: plainText,
			NormalizedTags: normalizedTags,
		}
		newDep := &cache.Dependencies{Tags: post.Tags}
		_ = s.cache.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*cache.Dependencies{postID: newDep})
	}

	cardRelPath := strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	imagePath := s.cfg.BaseURL + "/static/images/cards/" + cardRelPath
	if img, ok := metaData["image"].(string); ok {
		if s.cfg.CompressImages && !strings.HasPrefix(img, "http") {
			ext := filepath.Ext(img)
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
				img = img[:len(img)-len(ext)] + ".webp"
			}
		}
		imagePath = s.cfg.BaseURL + img
	}

	s.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: metaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
		TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
		TOC: toc, Config: s.cfg, SiteTree: siteTree,
		CurrentVersion: version, IsOutdated: s.isOutdatedVersion(version),
		Versions: s.cfg.GetVersionsMetadata(version, cleanHtmlRelPath),
		PrevPage: prev, NextPage: next,
	})

	return nil
}
