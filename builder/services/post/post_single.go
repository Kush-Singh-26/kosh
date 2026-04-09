package post

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/cache"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// ProcessSingle processes and renders a single markdown file.
func (s *postService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return s.ProcessSingleWithResult(ctx, path, source, nil)
}

type navResult struct {
	prev, next *models.NavPage
	allTags    []models.TagData
}

func (s *postService) resolveNavigation(post models.PostMetadata) *navResult {
	var posts []models.PostMetadata
	if s.cache != nil {
		if metas, err := s.cache.GetAllPostsMetadata(); err == nil {
			posts = make([]models.PostMetadata, len(metas))
			for i, m := range metas {
				posts[i] = models.PostMetadata{
					Title:   m.Title,
					Link:    m.Link,
					Weight:  m.Weight,
					DateObj: m.Date,
					Tags:    m.Tags,
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

	// Build all tags for the search modal even in incremental mode
	tagMap := make(map[string][]models.PostMetadata)
	for _, p := range posts {
		if p.Draft && !s.cfg.IncludeDrafts {
			continue
		}
		for _, t := range p.Tags {
			tagMap[t] = append(tagMap[t], p)
		}
	}
	allTags := generators.BuildAllTags(tagMap)

	return &navResult{prev: prev, next: next, allTags: allTags}
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
		newMath := make(map[string]any) // values are rendered HTML/SVG strings.
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

// ProcessSingleWithResult processes and renders a single markdown file using an optional pre-parse result.
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
	htmlRelPath, _, destPath := navigation.ComputePathVars(s.cfg.OutputDir, relPath)

	var parseRes *ParsedMarkdownResult
	if preParsed != nil {
		parseRes = preParsed
	} else {
		parseRes, err = ParseMarkdown(ParseOptions{
			Path:             path,
			RelPath:          relPath,
			Source:           source,
			Info:             info,
			Renderer:         s.renderer,
			NativeRenderer:   s.nativeRenderer,
			MdPool:           s.mdPool,
			DiagramAdapter:   s.diagramAdapter,
			Metrics:          s.metrics,
			Cfg:              s.cfg,
			CleanHtmlRelPath: htmlRelPath,
			HtmlRelPath:      htmlRelPath,
		})
		if err != nil {
			return err
		}
	}

	htmlContent := s.renderMathSSR(ctx, parseRes.HTMLContent, parseRes.MathExpressions)
	post := parseRes.Post
	nav := s.resolveNavigation(post)
	cardRelPath, cardDestPath, cardImageURL := navigation.CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, htmlRelPath)

	if s.cfg.Features.RawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		if err := s.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
			s.logger.Warn("Failed to create directory for raw markdown", "dir", filepath.Dir(mdDestPath), "error", err)
		}
		if err := s.sink.WriteFile(mdDestPath, source); err == nil {
			s.renderer.RegisterFile(mdDestPath)
		}
	}

	if s.cache != nil {
		s.commitPostCache(commitPostCacheOptions{
			parseRes:    parseRes,
			post:        post,
			relPath:     relPath,
			info:        info,
			htmlContent: htmlContent,
		})
		s.handleSocialCard(parseRes, relPath, cardRelPath, cardDestPath)
	}

	return s.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: parseRes.Metadata, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
		TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: cardImageURL,
		TOC: parseRes.TOC, Config: s.cfg, ReadingTime: post.ReadingTime,
		AllTags:  nav.allTags,
		PrevPage: nav.prev, NextPage: nav.next, RelativePrefix: fspkg.GetRelativePrefix(htmlRelPath),
		HasImages: parseRes.HasImages,
		JSONLD:    models.GeneratePostJSONLD(post, s.cfg.Author, cardImageURL),
	})
}

type commitPostCacheOptions struct {
	parseRes    *ParsedMarkdownResult
	post        models.PostMetadata
	relPath     string
	info        os.FileInfo
	htmlContent string
}

func (s *postService) commitPostCache(opts commitPostCacheOptions) {
	if opts.parseRes == nil {
		panic("commitPostCache: parseRes is nil")
	}
	if opts.info == nil {
		panic("commitPostCache: info is nil")
	}
	if opts.relPath == "" {
		panic("commitPostCache: relPath is empty")
	}

	postID := cache.GeneratePostID("", opts.relPath)
	cacheTOC := make([]models.TOCEntry, len(opts.parseRes.TOC))
	for i, t := range opts.parseRes.TOC {
		cacheTOC[i] = models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level}
	}

	newMeta := &cache.PostMeta{
		PostID: postID, Path: opts.relPath, ModTime: opts.info.ModTime().Unix(),
		ContentHash: opts.parseRes.FrontmatterHash, BodyHash: hashing.GetBodyHash(nil),
		Title: opts.post.Title, Date: opts.post.DateObj, Tags: opts.post.Tags,
		ReadingTime: opts.post.ReadingTime, Description: opts.post.Description,
		Link: opts.post.Link, Pinned: opts.post.Pinned, Weight: opts.post.Weight,
		Draft: opts.post.Draft, Meta: opts.parseRes.Metadata, TOC: cacheTOC,
		SSRInputHashes: opts.parseRes.SSRHashes,
		CardHash:       opts.parseRes.FrontmatterHash,
		HasImages:      opts.parseRes.HasImages,
	}
	if err := s.cache.StoreHTMLForPost(newMeta, []byte(opts.htmlContent)); err != nil {
		s.logger.Error("Failed to store HTML in cache", "path", opts.relPath, "error", err)
	}

	normalizedTags := make([]string, len(opts.post.Tags))
	for i, t := range opts.post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	newSearch := &cache.SearchRecord{
		Title:           opts.post.Title,
		NormalizedTitle: strings.ToLower(opts.post.Title),
		BM25Data:        opts.parseRes.WordFreqs,
		DocLen:          opts.parseRes.DocLen,
		Content:         opts.parseRes.PlainText,
		NormalizedTags:  normalizedTags,
		StemMap:         opts.parseRes.StemMap,
		PositionalIndex: opts.parseRes.PositionalIndex,
		ByteOffsets:     opts.parseRes.ByteOffsets,
	}
	newDep := &models.Dependencies{Tags: opts.post.Tags}

	s.cacheWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       context.Background(),
		Logger:    s.logger,
		Operation: "cache commit",
		Fn: func() error {
			timer := timeutil.StartPhase("Cache commit (incremental)")
			if err := s.cache.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*models.Dependencies{postID: newDep}); err != nil {
				s.logger.Error("Failed to commit post to cache", "path", opts.relPath, "error", err)
			}
			timer.Stop()
			return nil
		},
		Cleanup: s.cacheWg.Done,
	})
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
