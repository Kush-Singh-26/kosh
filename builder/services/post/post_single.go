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
	taxonomies map[string]models.TaxonomyData
}

func (service *postService) resolveNavigation(post models.PostMetadata) *navResult {
	var posts []models.PostMetadata
	if service.cache != nil {
		if metas, err := service.cache.GetAllPostsMetadata(); err == nil {
			posts = make([]models.PostMetadata, len(metas))
			for idx, meta := range metas {
				p := models.PostMetadata{
					Title:      meta.Title,
					Link:       meta.Link,
					Weight:     meta.Weight,
					Section:    meta.Section,
					DateObj:    meta.Date,
					Taxonomies: meta.Taxonomies,
				}
				posts[idx] = p
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

	// Build all taxonomies
	taxonomyMap := make(map[string]map[string][]models.PostMetadata)
	for _, p := range posts {
		if p.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		for taxKey, terms := range p.Taxonomies {
			if taxonomyMap[taxKey] == nil {
				taxonomyMap[taxKey] = make(map[string][]models.PostMetadata)
			}
			for _, t := range terms {
				taxonomyMap[taxKey][t] = append(taxonomyMap[taxKey][t], p)
			}
		}
	}

	taxonomies := make(map[string]models.TaxonomyData)
	for taxKey, plural := range service.cfg.Taxonomies {
		if termMap, ok := taxonomyMap[taxKey]; ok {
			taxonomies[taxKey] = models.TaxonomyData{
				Name:   taxKey,
				Plural: plural,
				Terms:  generators.BuildTaxonomyData(service.cfg.ContentPrefix, plural, termMap),
			}
		}
	}

	return &navResult{prev: prev, next: next, taxonomies: taxonomies}
}

func (service *postService) renderSSR(ctx context.Context, html string, result *ParsedMarkdownResult) string {
	if len(result.MathExpressions) == 0 && len(result.D2Expressions) == 0 {
		return html
	}

	// 1. Math
	if len(result.MathExpressions) > 0 {
		cached := make(map[string]string)
		if service.diagramAdapter != nil {
			for _, expr := range result.MathExpressions {
				key := "math:" + expr.Hash
				if val, ok := service.diagramAdapter.GetLocal(key); ok {
					if renderedStr, ok := val.(string); ok {
						cached[expr.Hash] = renderedStr
					}
				}
			}
		}

		rendered, err := service.nativeRenderer.RenderAllMath(ctx, result.MathExpressions, cached)
		if err == nil {
			if service.diagramAdapter != nil && len(rendered) > 0 {
				newMath := make(map[string]any)
				for hash, content := range rendered {
					if _, ok := cached[hash]; !ok {
						newMath["math:"+hash] = content
					}
				}
				if len(newMath) > 0 {
					service.diagramAdapter.Merge(newMath)
				}
			}
			html = mdParser.ReplaceMathExpressions(html, result.MathExpressions, rendered)
		}
	}

	// 2. D2
	if len(result.D2Expressions) > 0 {
		cached := make(map[string]models.SSRThemePair)
		if service.diagramAdapter != nil {
			for _, expr := range result.D2Expressions {
				key := "d2:" + expr.Hash
				if val, ok := service.diagramAdapter.GetLocal(key); ok {
					if pair, ok := val.(models.SSRThemePair); ok {
						cached[expr.Hash] = pair
					}
				}
			}
		}

		rendered, err := service.nativeRenderer.RenderAllD2(ctx, result.D2Expressions, cached)
		if err == nil {
			if service.diagramAdapter != nil && len(rendered) > 0 {
				newD2 := make(map[string]any)
				for hash, pair := range rendered {
					if _, ok := cached[hash]; !ok {
						newD2["d2:"+hash] = pair
					}
				}
				if len(newD2) > 0 {
					service.diagramAdapter.Merge(newD2)
				}
			}
			html = mdParser.ReplaceD2Expressions(html, result.D2Expressions, rendered)
		}
	}

	return html
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

	// Calculate Section
	section := ""
	cleanRel := filepath.ToSlash(relPath)
	cleanRel = strings.TrimPrefix(cleanRel, "./")
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	parts := strings.Split(cleanRel, "/")
	if len(parts) > 1 {
		section = parts[0]
	}
	service.logger.Info("Section detected", "relPath", relPath, "section", section)
	service.logger.Info("Calculating section for post", "relPath", relPath, "section", section)

	var parseRes *ParsedMarkdownResult
	if preParsed != nil {
		parseRes = preParsed
	} else {
		var err error
		parseRes, err = ParseMarkdown(ParseOptions{
			Path:             path,
			RelPath:          relPath,
			Source:           source,
			Info:             info,
			Renderer:         service.renderer,
			NativeRenderer:   service.nativeRenderer,
			MdPool:           service.mdPool,
			DiagramAdapter:   service.diagramAdapter,
			Metrics:          service.metrics,
			Cfg:              service.cfg,
			HtmlRelPath:      htmlRelPath,
			CleanHtmlRelPath: strings.TrimSuffix(htmlRelPath, "index.html"),
		})
		if err != nil {
			return err
		}
	}

	// Ensure section is set on the post early
	parseRes.Post.Section = section

	htmlContent := service.renderSSR(ctx, parseRes.HTMLContent, parseRes)
	relPrefix := fspkg.GetRelativePrefix(htmlRelPath)
	htmlContent = rewriteStaticAssetPaths(htmlContent, relPrefix)

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

	contentPrefix := strings.Trim(service.cfg.ContentPrefix, "/")
	postsIndexURL := "index.html"
	if contentPrefix != "" {
		postsIndexURL = "/" + contentPrefix + "/"
	} else if relPrefix != "" {
		postsIndexURL = relPrefix + "index.html"
	}

	// Determine if this is a blog page or a custom layout page
	// Pages with layout: "home" (portfolio) should NOT be treated as blog pages
	layoutVal := ""
	if l, ok := parseRes.Metadata["layout"].(string); ok {
		layoutVal = strings.ToLower(l)
	} else if l, ok := parseRes.Metadata["Layout"].(string); ok {
		layoutVal = strings.ToLower(l)
	}
	isContent := layoutVal != "home"
	pageContext := models.ContextPosts
	if !isContent {
		pageContext = models.ContextHome
	}

	return service.renderer.RenderPage(destPath, models.PageData{
		Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
		Meta: parseRes.Metadata, BaseURL: service.cfg.BaseURL, BuildVersion: service.cfg.BuildVersion,
		TabTitle: post.Title + " | " + service.cfg.Title, Permalink: post.Link, Image: cardImageURL,
		TOC: parseRes.TOC, Config: service.cfg, ReadingTime: post.ReadingTime,
		Taxonomies:     nav.taxonomies,
		ItemTaxonomies: post.Taxonomies,
		PrevPage:       nav.prev, NextPage: nav.next, RelativePrefix: relPrefix,
		HasImages: parseRes.HasImages, Context: pageContext,
		ContentPrefix: contentPrefix,
		PostsIndexURL: postsIndexURL,
		JSONLD:        service.generateJSONLD(post, cardImageURL),
		Section:       section,
		IsCleanBuild:  service.ctx.IsCleanBuild,
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
		PostID: postID, Path: options.relPath, ModTime: options.info.ModTime().UnixNano(),
		ContentHash: options.parseRes.FrontmatterHash, BodyHash: hashing.GetBodyHash(nil),
		Title: options.post.Title, Date: options.post.DateObj, Taxonomies: options.post.Taxonomies,
		ReadingTime: options.post.ReadingTime, Description: options.post.Description,
		Link: options.post.Link, IsPinned: options.post.IsPinned, Weight: options.post.Weight,
		Section: options.post.Section,
		IsDraft: options.post.IsDraft, Meta: options.parseRes.Metadata, TOC: cacheTOC,
		SSRInputHashes: options.parseRes.SSRHashes,
		CardHash:       options.parseRes.FrontmatterHash,
		HasImages:      options.parseRes.HasImages,
	}
	if err := service.cache.StoreHTMLForPost(newMeta, []byte(options.htmlContent)); err != nil {
		service.logger.Error("Failed to store HTML in cache", "path", options.relPath, "error", err)
	}

	newSearch := &cache.SearchRecord{
		Title:           options.post.Title,
		NormalizedTitle: strings.ToLower(options.post.Title),
		BM25Data:        options.parseRes.WordFreqs,
		DocLen:          options.parseRes.DocLen,
		Content:         options.parseRes.PlainText,
		Taxonomies:      options.parseRes.SearchRecord.Taxonomies,
		NormalizedTaxs:  options.parseRes.SearchRecord.NormalizedTaxs,
		StemMap:         options.parseRes.StemMap,
		PositionalIndex: options.parseRes.PositionalIndex,
		ByteOffsets:     options.parseRes.ByteOffsets,
	}
	newDep := &models.Dependencies{Taxonomies: options.post.Taxonomies}

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
