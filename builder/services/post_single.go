package services

import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (s *postServiceImpl) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return s.ProcessSingleWithResult(ctx, path, source, nil)
}

func (s *postServiceImpl) ProcessSingleWithResult(ctx context.Context, path string, source []byte, preParsed *ParsedMarkdownResult) error {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("PANIC recovered in ProcessSingle",
				"path", path,
				"error", r,
				"stack", string(debug.Stack()))
		}
	}()

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

	// source is already read and passed in to avoid TOCTOU race condition
	if source == nil {
		source, err = afero.ReadFile(s.sourceFs, path)
		if err != nil {
			s.logger.Error("Error reading file", "path", path, "error", err)
			return err
		}
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

	var parseRes *ParsedMarkdownResult
	if preParsed != nil {
		parseRes = preParsed
	} else {
		// Parse if not provided
		parseRes, err = ParseMarkdown(
			ctx,
			source,
			path,
			version,
			cleanHtmlRelPath,
			htmlRelPath,
			s.mdPool,
			s.cfg,
			s.nativeRenderer,
			s.diagramAdapter,
			&s.mu,
			"",
			0,
			0,
			nil,
		)
		if err != nil {
			return err
		}
	}

	htmlContent := parseRes.HTMLContent
	if len(parseRes.MathExpressions) > 0 {
		cachedSubset := make(map[string]string)
		if s.diagramAdapter != nil {
			for _, expr := range parseRes.MathExpressions {
				if v, ok := s.diagramAdapter.GetLocal(expr.Hash); ok {
					cachedSubset[expr.Hash] = v
				}
			}
		}

		rendered, err := s.nativeRenderer.RenderAllMath(ctx, parseRes.MathExpressions, cachedSubset)
		if err != nil {
			s.logger.Warn("Math render failed for post", "path", path, "error", err)
		}

		if s.diagramAdapter != nil && len(rendered) > 0 {
			newMath := make(map[string]string)
			for h, v := range rendered {
				if _, ok := cachedSubset[h]; !ok {
					newMath[h] = v
				}
			}
			if len(newMath) > 0 {
				s.diagramAdapter.Merge(newMath)
			}
		}

		htmlContent = mdParser.ReplaceMathExpressions(htmlContent, parseRes.MathExpressions, rendered)
	}
	metaData := parseRes.MetaData
	post := parseRes.Post
	toc := parseRes.TOC
	ssrHashes := parseRes.SSRHashes
	wordFreqs := parseRes.WordFreqs
	docLen := parseRes.DocLen
	stemMap := parseRes.StemMap
	posIndex := parseRes.PositionalIndex

	if s.cfg.Features.RawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		_ = s.sink.MkdirAll(filepath.Dir(mdDestPath))
		if err := s.sink.WriteFile(mdDestPath, source); err == nil {
			s.renderer.RegisterFile(mdDestPath)
		}
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

	normalizedTags := make([]string, len(post.Tags))
	for i, t := range post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	if s.cache != nil {
		postID := cache.GeneratePostID("", relPath)
		cacheTOC := make([]models.TOCEntry, len(toc))
		for i, t := range toc {
			cacheTOC[i] = models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level}
		}

		frontmatterHash := parseRes.FrontmatterHash
		bodyHash := utils.GetBodyHash(source)

		newMeta := &cache.PostMeta{
			PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
			ContentHash: frontmatterHash, BodyHash: bodyHash,
			Title: post.Title, Date: post.DateObj, Tags: post.Tags,
			ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight,
			Draft: post.Draft, Meta: metaData, TOC: cacheTOC, Version: version,
			SSRInputHashes: ssrHashes,
			CardHash:       frontmatterHash,
			HasImages:      parseRes.HasImages,
		}
		// Use StoreHTMLForPost to properly inline small posts (<32KB), consistent with full builds
		if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
			s.logger.Error("Failed to store HTML in cache", "path", relPath, "error", err)
		}

		newSearch := &cache.SearchRecord{
			Title:           post.Title,
			NormalizedTitle: strings.ToLower(post.Title),
			BM25Data:        wordFreqs,
			DocLen:          docLen,
			Content:         parseRes.PlainText,
			NormalizedTags:  normalizedTags,
			StemMap:         stemMap,
			PositionalIndex: posIndex,
		}
		newDep := &cache.Dependencies{Tags: post.Tags}

		// Fire cache commit in the background -- best-effort, errors logged only.
		commitMeta := []*cache.PostMeta{newMeta}
		commitSearch := map[string]*cache.SearchRecord{postID: newSearch}
		commitDeps := map[string]*cache.Dependencies{postID: newDep}

		s.cacheWg.Add(1)
		go func() {
			defer s.cacheWg.Done()
			cacheCommitTimer := utils.StartPhase("Cache commit (incremental)")
			if err := s.cache.BatchCommit(commitMeta, commitSearch, commitDeps); err != nil {
				s.logger.Error("Failed to commit post to cache", "path", path, "error", err)
			}
			cacheCommitTimer.Stop()
		}()
		// 2. Generate/Copy Social Card
		cardRelPath := strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
		cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", cardRelPath))

		cachedHash, _ := s.cache.GetSocialCardHash(relPath)
		cardExists := cachedHash != "" && cachedHash == parseRes.FrontmatterHash
		if !cardExists {
			if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err == nil {
				s.generateSocialCard(socialCardTask{
					path:            relPath,
					relPath:         cardRelPath,
					cardDestPath:    cardDestPath,
					metaData:        metaData,
					frontmatterHash: parseRes.FrontmatterHash,
				})
			}
		}
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
		PrevPage: prev, NextPage: next, HasImages: parseRes.HasImages,
	})

	return nil
}
