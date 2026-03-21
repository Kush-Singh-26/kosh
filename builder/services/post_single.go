package services

import (
	"context"
	"fmt"
	"html/template"
	"os"
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

type navResult struct {
	prev, next *models.NavPage
	siteTree   []*models.TreeNode
}

func (s *postService) resolveNavigation(post models.PostMetadata, version string) *navResult {
	var posts []models.PostMetadata
	if s.cache != nil {
		if metas, err := s.cache.GetPostsMetadataByVersion(version); err == nil {
			posts = make([]models.PostMetadata, len(metas))
			for i, m := range metas {
				posts[i] = models.PostMetadata{
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
	for i, p := range posts {
		if p.Link == post.Link {
			posts[i] = post
			found = true
			break
		}
	}
	if !found {
		posts = append(posts, post)
	}

	timeutil.SortPosts(posts)
	prev, next, _ := navigation.FindPrevNext(post, posts)
	siteTree := fspkg.BuildSiteTree(posts, post.Link)

	return &navResult{prev: prev, next: next, siteTree: siteTree}
}

func (s *postService) renderMathSSR(ctx context.Context, html string, exprs []models.MathExpression) string {
	if len(exprs) == 0 {
		return html
	}

	cached := make(map[string]string)
	if s.diagramAdapter != nil {
		for _, expr := range exprs {
			key := "math:" + expr.Hash
			if v, ok := s.diagramAdapter.GetLocal(key); ok {
				if s, ok := v.(string); ok {
					cached[expr.Hash] = s
				}
			}
		}
	}

	rendered, err := s.nativeRenderer.RenderAllMath(ctx, exprs, cached)
	if err != nil {
		s.logger.Warn("Math render failed", "error", err)
		return html
	}

	if s.diagramAdapter != nil && len(rendered) > 0 {
		newMath := make(map[string]any)
		for h, v := range rendered {
			if _, ok := cached[h]; !ok {
				newMath["math:"+h] = v
			}
		}
		if len(newMath) > 0 {
			s.diagramAdapter.Merge(newMath)
		}
	}

	return mdParser.ReplaceMathExpressions(html, exprs, rendered)
}

func (s *postService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, preParsed *ParsedMarkdownResult) error {
	info, err := s.sourceFs.Stat(path)
	if err != nil {
		s.logger.Error("Error stating file", "path", path, "error", err)
		return err
	}

	if info.Size() > models.MaxFileSize {
		s.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", models.MaxFileSize)
		return fmt.Errorf("file size %d exceeds limit %d", info.Size(), models.MaxFileSize)
	}

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

	htmlContent := s.renderMathSSR(ctx, parseRes.HTMLContent, parseRes.MathExpressions)
	post := parseRes.Post
	nav := s.resolveNavigation(post, version)
	cardRelPath, cardDestPath, cardImageURL := CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, htmlRelPath)

	if s.cfg.Features.RawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		_ = s.sink.MkdirAll(filepath.Dir(mdDestPath))
		if err := s.sink.WriteFile(mdDestPath, source); err == nil {
			s.renderer.RegisterFile(mdDestPath)
		}
	}

	if s.cache != nil {
		s.commitPostCache(parseRes, post, relPath, version, info, htmlContent)
		s.handleSocialCard(parseRes, relPath, cardRelPath, cardDestPath)
	}

	return s.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: parseRes.Metadata, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
		TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: cardImageURL,
		TOC: parseRes.TOC, Config: s.cfg, CurrentVersion: version, ReadingTime: post.ReadingTime,
		PrevPage: nav.prev, NextPage: nav.next, RelativePrefix: fspkg.GetRelativePrefix(htmlRelPath),
		HasImages: parseRes.HasImages, SiteTree: nav.siteTree,
		JSONLD: models.GeneratePostJSONLD(post, s.cfg.Author),
	})
}

func (s *postService) commitPostCache(parseRes *ParsedMarkdownResult, post models.PostMetadata, relPath, version string, info os.FileInfo, htmlContent string) {
	postID := cache.GeneratePostID("", relPath)
	cacheTOC := make([]models.TOCEntry, len(parseRes.TOC))
	for i, t := range parseRes.TOC {
		cacheTOC[i] = models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level}
	}

	newMeta := &cache.PostMeta{
		PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
		ContentHash: parseRes.FrontmatterHash, BodyHash: hashing.GetBodyHash(nil),
		Title: post.Title, Date: post.DateObj, Tags: post.Tags,
		ReadingTime: post.ReadingTime, Description: post.Description,
		Link: post.Link, Pinned: post.Pinned, Weight: post.Weight,
		Draft: post.Draft, Meta: parseRes.Metadata, TOC: cacheTOC, Version: version,
		SSRInputHashes: parseRes.SSRHashes,
		CardHash:       parseRes.FrontmatterHash,
		HasImages:      parseRes.HasImages,
	}
	if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
		s.logger.Error("Failed to store HTML in cache", "path", relPath, "error", err)
	}

	normalizedTags := make([]string, len(post.Tags))
	for i, t := range post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	newSearch := &cache.SearchRecord{
		Title:           post.Title,
		NormalizedTitle: strings.ToLower(post.Title),
		BM25Data:        parseRes.WordFreqs,
		DocLen:          parseRes.DocLen,
		Content:         parseRes.PlainText,
		NormalizedTags:  normalizedTags,
		StemMap:         parseRes.StemMap,
		PositionalIndex: parseRes.PositionalIndex,
	}
	newDep := &cache.Dependencies{Tags: post.Tags}

	s.cacheWg.Add(1)
	go func() {
		defer s.cacheWg.Done()
		timer := timeutil.StartPhase("Cache commit (incremental)")
		if err := s.cache.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*cache.Dependencies{postID: newDep}); err != nil {
			s.logger.Error("Failed to commit post to cache", "path", relPath, "error", err)
		}
		timer.Stop()
	}()
}

func (s *postService) handleSocialCard(parseRes *ParsedMarkdownResult, relPath, cardRelPath, cardDestPath string) {
	cachedHash, _ := s.cache.GetSocialCardHash(relPath)
	if cachedHash != "" && cachedHash == parseRes.FrontmatterHash {
		s.sink.Register(cardDestPath)
		return
	}
	if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
		return
	}
	s.generateSocialCard(socialCardTask{
		path:            relPath,
		relPath:         cardRelPath,
		cardDestPath:    cardDestPath,
		metadata:        parseRes.Metadata,
		frontmatterHash: parseRes.FrontmatterHash,
	})
}
