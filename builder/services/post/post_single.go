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
func (service *postService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return service.ProcessSingleWithResult(ctx, path, source, nil)
}

type navResult struct {
	prev, next *models.NavPage
	allTags    []models.TagData
}

func (service *postService) resolveNavigation(post models.PostMetadata) *navResult {
	var posts []models.PostMetadata
	if service.cache != nil {
		if metas, err := service.cache.GetAllPostsMetadata(); err == nil {
			posts = make([]models.PostMetadata, len(metas))
			for idx, meta := range metas {
				posts[idx] = models.PostMetadata{
					Title:   meta.Title,
					Link:    meta.Link,
					Weight:  meta.Weight,
					DateObj: meta.Date,
					Tags:    meta.Tags,
				}
			}
		}
	}

	found := false
	for idx, postObj := range posts {
		if postObj.Link == post.Link {
			posts[idx] = post
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
		if p.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		for _, t := range p.Tags {
			tagMap[t] = append(tagMap[t], p)
		}
	}
	allTags := generators.BuildAllTags(tagMap)

	return &navResult{prev: prev, next: next, allTags: allTags}
}

func (service *postService) renderMathSSR(ctx context.Context, html string, expressions []models.MathExpression) string {
	if len(expressions) == 0 {
		return html
	}

	cached := make(map[string]string)
	if service.diagramAdapter != nil {
		for _, expr := range expressions {
			key := "math:" + expr.Hash
			if val, ok := service.diagramAdapter.GetLocal(key); ok {
				if renderedStr, ok := val.(string); ok {
					cached[expr.Hash] = renderedStr
				}
			}
		}
	}

	rendered, err := service.nativeRenderer.RenderAllMath(ctx, expressions, cached)
	if err != nil {
		service.logger.Warn("Math render failed", "error", err)
		return html
	}

	if service.diagramAdapter != nil && len(rendered) > 0 {
		newMath := make(map[string]any) // values are rendered HTML/SVG strings.
		for hash, content := range rendered {
			if _, ok := cached[hash]; !ok {
				newMath["math:"+hash] = content
			}
		}
		if len(newMath) > 0 {
			service.diagramAdapter.Merge(newMath)
		}
	}

	return mdParser.ReplaceMathExpressions(html, expressions, rendered)
}

// ProcessSingleWithResult processes and renders a single markdown file using an optional pre-parse result.
func (service *postService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, preParsed *ParsedMarkdownResult) error {
	info, err := service.sourceFs.Stat(path)
	if err != nil {
		service.logger.Error("Error stating file", "path", path, "error", err)
		return err
	}

	if info.Size() > models.MaxFileSize {
		service.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", models.MaxFileSize)
		return fmt.Errorf("file size %d exceeds limit %d", info.Size(), models.MaxFileSize)
	}

	if source == nil {
		source, err = afero.ReadFile(service.sourceFs, path)
		if err != nil {
			service.logger.Error("Error reading file", "path", path, "error", err)
			return err
		}
	}

	relPath, _ := fspkg.SafeRel(service.cfg.ContentDir, path)
	htmlRelPath, _, destPath := navigation.ComputePathVars(service.cfg.OutputDir, relPath)

	var parseRes *ParsedMarkdownResult
	if preParsed != nil {
		parseRes = preParsed
	} else {
		var err error
		parseRes, err = ParseMarkdown(ParseOptions{
			Path:            path,
			RelPath:         relPath,
			Source:          source,
			Info:            info,
			Renderer:        service.renderer,
			NativeRenderer:  service.nativeRenderer,
			MdPool:          service.mdPool,
			DiagramAdapter:  service.diagramAdapter,
			Metrics:         service.metrics,
			Cfg:             service.cfg,
			HtmlRelPath:     htmlRelPath,
			CleanHtmlRelPath: strings.TrimSuffix(htmlRelPath, "index.html"),
		})
		if err != nil {
			return err
		}
	}

	htmlContent := service.renderMathSSR(ctx, parseRes.HTMLContent, parseRes.MathExpressions)
	post := parseRes.Post
	nav := service.resolveNavigation(post)
	cardRelPath, cardDestPath, cardImageURL := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelPath)

	if service.cfg.Features.UseRawMarkdown {
		mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
		if err := service.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
			service.logger.Warn("Failed to create directory for raw markdown", "dir", filepath.Dir(mdDestPath), "error", err)
		}
		if err := service.sink.WriteFile(mdDestPath, source); err == nil {
			service.renderer.RegisterFile(mdDestPath)
		}
	}

	if service.cache != nil {
		service.commitPostCache(commitPostCacheOptions{
			parseRes:    parseRes,
			post:        post,
			relPath:     relPath,
			info:        info,
			htmlContent: htmlContent,
		})
		service.handleSocialCard(parseRes, relPath, cardRelPath, cardDestPath)
	}

	return service.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: parseRes.Metadata, BaseURL: service.cfg.BaseURL, BuildVersion: service.cfg.BuildVersion,
		TabTitle: post.Title + " | " + service.cfg.Title, Permalink: post.Link, Image: cardImageURL,
		TOC: parseRes.TOC, Config: service.cfg, ReadingTime: post.ReadingTime,
		AllTags:  nav.allTags,
		PrevPage: nav.prev, NextPage: nav.next, RelativePrefix: fspkg.GetRelativePrefix(htmlRelPath),
		HasImages: parseRes.HasImages,
		JSONLD:    service.generateJSONLD(post, cardImageURL),
	})
}

type commitPostCacheOptions struct {
	parseRes    *ParsedMarkdownResult
	post        models.PostMetadata
	relPath     string
	info        os.FileInfo
	htmlContent string
}

func (service *postService) commitPostCache(options commitPostCacheOptions) {
	if options.parseRes == nil {
		panic("commitPostCache: parseRes is nil")
	}
	if options.info == nil {
		panic("commitPostCache: info is nil")
	}
	if options.relPath == "" {
		panic("commitPostCache: relPath is empty")
	}

	postID := cache.GeneratePostID("", options.relPath)
	cacheTOC := make([]models.TOCEntry, len(options.parseRes.TOC))
	for idx, tocEntry := range options.parseRes.TOC {
		cacheTOC[idx] = models.TOCEntry{ID: tocEntry.ID, Text: tocEntry.Text, Level: tocEntry.Level}
	}

	newMeta := &cache.PostMeta{
		PostID: postID, Path: options.relPath, ModTime: options.info.ModTime().Unix(),
		ContentHash: options.parseRes.FrontmatterHash, BodyHash: hashing.GetBodyHash(nil),
		Title: options.post.Title, Date: options.post.DateObj, Tags: options.post.Tags,
		ReadingTime: options.post.ReadingTime, Description: options.post.Description,
		Link: options.post.Link, IsPinned: options.post.IsPinned, Weight: options.post.Weight,
		IsDraft: options.post.IsDraft, Meta: options.parseRes.Metadata, TOC: cacheTOC,
		SSRInputHashes: options.parseRes.SSRHashes,
		CardHash:       options.parseRes.FrontmatterHash,
		HasImages:      options.parseRes.HasImages,
	}
	if err := service.cache.StoreHTMLForPost(newMeta, []byte(options.htmlContent)); err != nil {
		service.logger.Error("Failed to store HTML in cache", "path", options.relPath, "error", err)
	}

	normalizedTags := make([]string, len(options.post.Tags))
	for idx, tag := range options.post.Tags {
		normalizedTags[idx] = strings.ToLower(tag)
	}

	newSearch := &cache.SearchRecord{
		Title:           options.post.Title,
		NormalizedTitle: strings.ToLower(options.post.Title),
		BM25Data:        options.parseRes.WordFreqs,
		DocLen:          options.parseRes.DocLen,
		Content:         options.parseRes.PlainText,
		NormalizedTags:  normalizedTags,
		StemMap:         options.parseRes.StemMap,
		PositionalIndex: options.parseRes.PositionalIndex,
		ByteOffsets:     options.parseRes.ByteOffsets,
	}
	newDep := &models.Dependencies{Tags: options.post.Tags}

	service.cacheWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       context.Background(),
		Logger:    service.logger,
		Operation: "cache commit",
		Fn: func() error {
			timer := timeutil.StartPhase("Cache commit (incremental)")
			if err := service.cache.BatchCommit([]*cache.PostMeta{newMeta}, map[string]*cache.SearchRecord{postID: newSearch}, map[string]*models.Dependencies{postID: newDep}); err != nil {
				service.logger.Error("Failed to commit post to cache", "path", options.relPath, "error", err)
			}
			timer.Stop()
			return nil
		},
		Cleanup: service.cacheWg.Done,
	})
}

func (service *postService) handleSocialCard(parseRes *ParsedMarkdownResult, relPath, cardRelPath, cardDestPath string) {
	cachedHash, _ := service.cache.GetSocialCardHash(relPath)
	if cachedHash != "" && cachedHash == parseRes.FrontmatterHash {
		service.sink.Register(cardDestPath)
		return
	}
	if err := service.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
		return
	}
	service.generateSocialCard(socialCardTask{
		path:            relPath,
		relPath:         cardRelPath,
		cardDestPath:    cardDestPath,
		metadata:        parseRes.Metadata,
		frontmatterHash: parseRes.FrontmatterHash,
	})
}
