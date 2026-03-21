package services

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

func (s *postService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return s.ProcessSingleWithResult(ctx, path, source, nil)
}

func (s *postService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, preParsed *ParsedMarkdownResult) error {
	info, err := s.sourceFs.Stat(path)
	if err != nil {
		s.logger.Error("Error stating file", "path", path, "error", err)
		return err
	}

	// Check file size before loading into memory
	if info.Size() > models.MaxFileSize {
		s.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", models.MaxFileSize)
		return fmt.Errorf("file size %d exceeds limit %d", info.Size(), models.MaxFileSize)
	}

	// source is already read and passed in to avoid TOCTOU race condition
	if source == nil {
		source, err = afero.ReadFile(s.sourceFs, path)
		if err != nil {
			s.logger.Error("Error reading file", "path", path, "error", err)
			return err
		}
	}

	relPath, _ := fspkg.SafeRel(s.cfg.ContentDir, path)
	version, _ := navigation.GetVersionFromPath(path)

	htmlRelPath, cleanHtmlRelPath, destPath := ComputePathVars(s.cfg.OutputDir, relPath, version)

	var parseRes *ParsedMarkdownResult
	if preParsed != nil {
		parseRes = preParsed
	} else {
		// Parse if not provided
		parseRes, err = ParseMarkdown(
			ParseConfig{
				Source:           source,
				Path:             path,
				Version:          version,
				CleanHtmlRelPath: cleanHtmlRelPath,
				HtmlRelPath:      htmlRelPath,
			},
			ParseContext{
				MdPool:         s.mdPool,
				Cfg:            s.cfg,
				NativeRenderer: s.nativeRenderer,
				DiagramAdapter: s.diagramAdapter,
				MathBatchSize:  DefaultMathBatchSize,
			},
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
				key := "math:" + expr.Hash
				if v, ok := s.diagramAdapter.GetLocal(key); ok {
					if s, ok := v.(string); ok {
						cachedSubset[expr.Hash] = s
					}
				}
			}
		}

		rendered, err := s.nativeRenderer.RenderAllMath(ctx, parseRes.MathExpressions, cachedSubset)
		if err != nil {
			s.logger.Warn("Math render failed for post", "path", path, "error", err)
		}

		if s.diagramAdapter != nil && len(rendered) > 0 {
			newMath := make(map[string]any)
			for h, v := range rendered {
				if _, ok := cachedSubset[h]; !ok {
					key := "math:" + h
					newMath[key] = v
				}
			}
			if len(newMath) > 0 {
				s.diagramAdapter.Merge(newMath)
			}
		}

		htmlContent = mdParser.ReplaceMathExpressions(htmlContent, parseRes.MathExpressions, rendered)
	}
	metadata := parseRes.Metadata
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

	timeutil.SortPosts(versionPosts)
	prev, next, err := navigation.FindPrevNext(post, versionPosts)
	if err != nil {
		s.logger.Debug("Navigation resolution failed", "path", path, "error", err)
	}
	siteTree := fspkg.BuildSiteTree(versionPosts, post.Link)
	cardRelPath, cardDestPath, cardImageURL := CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, htmlRelPath)

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
		bodyHash := hashing.GetBodyHash(source)

		newMeta := &cache.PostMeta{
			PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
			ContentHash: frontmatterHash, BodyHash: bodyHash,
			Title: post.Title, Date: post.DateObj, Tags: post.Tags,
			ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight,
			Draft: post.Draft, Meta: metadata, TOC: cacheTOC, Version: version,
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
			cacheCommitTimer := timeutil.StartPhase("Cache commit (incremental)")
			if err := s.cache.BatchCommit(commitMeta, commitSearch, commitDeps); err != nil {
				s.logger.Error("Failed to commit post to cache", "path", path, "error", err)
			}
			cacheCommitTimer.Stop()
		}()
		// 2. Generate/Copy Social Card
		cachedHash, _ := s.cache.GetSocialCardHash(relPath)
		cardExists := cachedHash != "" && cachedHash == parseRes.FrontmatterHash
		if !cardExists {
			if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
				return err
			}
			s.generateSocialCard(socialCardTask{
				path:            relPath,
				relPath:         cardRelPath,
				cardDestPath:    cardDestPath,
				metadata:        metadata,
				frontmatterHash: frontmatterHash,
			})
		} else {
			s.sink.Register(cardDestPath)
		}
	}

	if err := s.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: metadata, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
		TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: cardImageURL,
		TOC: toc, Config: s.cfg, CurrentVersion: version, ReadingTime: post.ReadingTime,
		PrevPage: prev, NextPage: next, RelativePrefix: fspkg.GetRelativePrefix(htmlRelPath),
		HasImages: parseRes.HasImages, SiteTree: siteTree,
		JSONLD: models.GeneratePostJSONLD(post, s.cfg.Author),
	}); err != nil {
		return err
	}
	return nil
}
