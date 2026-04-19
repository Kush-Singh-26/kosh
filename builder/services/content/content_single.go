package content

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
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
func (service *contentService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return service.ProcessSingleWithResult(ctx, path, source, nil)
}

type navResult struct {
	prev, next *models.NavPage
	taxonomies map[string]models.TaxonomyData
}

func (service *contentService) resolveNavigation(item models.ContentMetadata) *navResult {
	var items []models.ContentMetadata
	if service.cache != nil {
		if metas, err := service.cache.GetAllItemsMetadata(); err == nil {
			items = make([]models.ContentMetadata, len(metas))
			for idx, meta := range metas {
				p := models.ContentMetadata{
					Title:      meta.Title,
					Link:       meta.Link,
					Weight:     meta.Weight,
					Section:    meta.Section,
					DateObj:    meta.Date,
					Taxonomies: meta.Taxonomies,
				}
				items[idx] = p
			}
		}
	}

	found := false
	for idx, itemObj := range items {
		if itemObj.Link == item.Link {
			items[idx] = item
			found = true
			break
		}
	}
	if !found {
		items = append(items, item)
	}

	timeutil.SortItems(items)
	prev, next, _ := navigation.FindPrevNext(item, items)

	// Build all taxonomies
	taxonomyMap := make(map[string]map[string][]models.ContentMetadata)
	for _, p := range items {
		if p.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		for taxKey, terms := range p.Taxonomies {
			if taxonomyMap[taxKey] == nil {
				taxonomyMap[taxKey] = make(map[string][]models.ContentMetadata)
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

func (service *contentService) renderSSR(ctx context.Context, html string, result *ParsedMarkdownResult) (string, map[string]string, map[string]models.SSRThemePair) {
	if len(result.MathExpressions) == 0 && len(result.D2Expressions) == 0 {
		return html, nil, nil
	}

	html, renderedMath := service.renderMathSSR(ctx, html, result)
	html, renderedD2 := service.renderD2SSR(ctx, html, result)

	return html, renderedMath, renderedD2
}

func (service *contentService) renderMathSSR(ctx context.Context, html string, result *ParsedMarkdownResult) (string, map[string]string) {
	if len(result.MathExpressions) == 0 {
		return html, nil
	}

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
	if err != nil {
		return html, nil
	}

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
	return mdParser.ReplaceMathExpressions(html, result.MathExpressions, rendered), rendered
}

func (service *contentService) renderD2SSR(ctx context.Context, html string, result *ParsedMarkdownResult) (string, map[string]models.SSRThemePair) {
	if len(result.D2Expressions) == 0 {
		return html, nil
	}

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
	if err != nil {
		return html, nil
	}

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
	return mdParser.ReplaceD2Expressions(html, result.D2Expressions, rendered), rendered
}

// ProcessSingleWithResult processes and renders a single markdown file using an optional pre-parse result.
func (service *contentService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, preParsed *ParsedMarkdownResult) error {
	info, source, err := service.validateAndReadSource(path, source)
	if err != nil {
		return err
	}

	relPath, _ := fspkg.SafeRel(service.cfg.ContentDir, path)
	htmlRelPath, _, destPath := navigation.ComputePathVars(service.cfg.OutputDir, relPath)
	section := detectSection(relPath, service.logger)

	parseRes, err := service.ensureParsed(ctx, path, relPath, htmlRelPath, source, info, preParsed)
	if err != nil {
		return err
	}
	parseRes.Item.Section = section

	htmlContent, renderedMath, renderedD2 := service.renderSSR(ctx, parseRes.HTMLContent, parseRes)
	relPrefix := fspkg.GetRelativePrefix(htmlRelPath)
	htmlContent = rewriteStaticAssetPaths(htmlContent, relPrefix)

	service.handleRawMarkdown(destPath, source)

	if service.cache != nil {
		service.commitContentCache(commitContentCacheOptions{
			parseRes:    parseRes,
			item:        parseRes.Item,
			relPath:     relPath,
			info:        info,
			htmlContent: htmlContent,
		})
		cardRelPath, cardDestPath, _ := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelPath)
		service.handleSocialCard(parseRes, relPath, cardRelPath, cardDestPath)
	}

	return service.renderFinalPage(destPath, htmlContent, relPrefix, htmlRelPath, section, parseRes, renderedMath, renderedD2)
}

func (service *contentService) validateAndReadSource(path string, source []byte) (os.FileInfo, []byte, error) {
	info, err := service.sourceFs.Stat(path)
	if err != nil {
		service.logger.Error("Error stating file", "path", path, "error", err)
		return nil, nil, err
	}

	if info.Size() > models.MaxFileSize {
		service.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", models.MaxFileSize)
		return nil, nil, fmt.Errorf("file size %d exceeds limit %d", info.Size(), models.MaxFileSize)
	}

	if source == nil {
		source, err = afero.ReadFile(service.sourceFs, path)
		if err != nil {
			service.logger.Error("Error reading file", "path", path, "error", err)
			return nil, nil, err
		}
	}
	return info, source, nil
}

func detectSection(relPath string, logger *slog.Logger) string {
	section := ""
	cleanRel := filepath.ToSlash(relPath)
	cleanRel = strings.TrimPrefix(cleanRel, "./")
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	parts := strings.Split(cleanRel, "/")
	if len(parts) > 1 {
		section = parts[0]
	}
	logger.Debug("Section detected", "relPath", relPath, "section", section)
	return section
}

func (service *contentService) ensureParsed(ctx context.Context, path, relPath, htmlRelPath string, source []byte, info os.FileInfo, preParsed *ParsedMarkdownResult) (*ParsedMarkdownResult, error) {
	if preParsed != nil {
		return preParsed, nil
	}
	if service.shortcodes != nil && len(source) > 0 {
		processed, err := service.shortcodes.Process(source)
		if err == nil {
			source = processed
		} else {
			service.logger.Warn("Shortcode processing failed", "path", path, "error", err)
		}
	}
	return ParseMarkdown(ctx, ParseOptions{
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
		HTMLRelPath:      htmlRelPath,
		CleanHTMLRelPath: strings.TrimSuffix(htmlRelPath, "index.html"),
	})
}

func (service *contentService) handleRawMarkdown(destPath string, source []byte) {
	if !service.cfg.Features.UseRawMarkdown {
		return
	}
	mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
	if err := service.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
		service.logger.Warn("Failed to create directory for raw markdown", "dir", filepath.Dir(mdDestPath), "error", err)
	}
	if err := service.sink.WriteFile(mdDestPath, source); err == nil {
		service.renderer.RegisterFile(mdDestPath)
	}
}

func (service *contentService) renderFinalPage(destPath, htmlContent, relPrefix, htmlRelPath, section string, parseRes *ParsedMarkdownResult, renderedMath map[string]string, renderedD2 map[string]models.SSRThemePair) error {
	item := parseRes.Item
	nav := service.resolveNavigation(item)
	_, _, cardImageURL := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelPath)

	contentPrefix := strings.Trim(service.cfg.ContentPrefix, "/")
	sectionIndexURL := service.getSectionIndexURL(relPrefix, contentPrefix)

	pageContext := models.ContextSection
	if getLayoutVal(parseRes.Metadata) == "home" {
		pageContext = models.ContextHome
	}

	return service.renderer.RenderPage(destPath, models.PageData{
		Title: item.Title, Description: item.Description, Content: template.HTML(htmlContent),
		Meta: parseRes.Metadata, BaseURL: service.cfg.BaseURL, BuildVersion: service.cfg.BuildVersion,
		TabTitle: item.Title + " | " + service.cfg.Title, Permalink: item.Link, Image: cardImageURL,
		TOC: parseRes.TOC, Config: service.cfg, ReadingTime: item.ReadingTime,
		Taxonomies:     nav.taxonomies,
		ItemTaxonomies: item.Taxonomies,
		PrevPage:       nav.prev, NextPage: nav.next, RelativePrefix: relPrefix,
		HasImages: parseRes.HasImages, Context: pageContext,
		ContentPrefix:   contentPrefix,
		SectionIndexURL: sectionIndexURL,
		JSONLD:          service.generateJSONLD(item, cardImageURL),
		Section:         section,
		IsCleanBuild:    service.ctx.IsCleanBuild,
		SSRMath:         renderedMath,
		SSRD2:           renderedD2,
	})
}

func (service *contentService) getSectionIndexURL(relPrefix, contentPrefix string) string {
	if contentPrefix != "" {
		return "/" + contentPrefix + "/"
	}
	if relPrefix != "" {
		return relPrefix + "index.html"
	}
	return "index.html"
}

func getLayoutVal(metadata map[string]any) string {
	if l, ok := metadata["layout"].(string); ok {
		return strings.ToLower(l)
	}
	if l, ok := metadata["Layout"].(string); ok {
		return strings.ToLower(l)
	}
	return ""
}

type commitContentCacheOptions struct {
	parseRes    *ParsedMarkdownResult
	item        models.ContentMetadata
	relPath     string
	info        os.FileInfo
	htmlContent string
}

func (service *contentService) commitContentCache(options commitContentCacheOptions) {
	if options.parseRes == nil {
		panic("commitContentCache: parseRes is nil")
	}
	if options.info == nil {
		panic("commitContentCache: info is nil")
	}
	if options.relPath == "" {
		panic("commitContentCache: relPath is empty")
	}

	ContentID := cache.GenerateContentID("", options.relPath)
	newMeta := service.buildCacheMeta(options, ContentID)

	if err := service.cache.StoreHTMLForItem(newMeta, []byte(options.htmlContent)); err != nil {
		service.logger.Error("Failed to store HTML in cache", "path", options.relPath, "error", err)
	}

	newSearch := service.buildSearchRecord(options.parseRes)
	newDep := &models.Dependencies{Taxonomies: options.item.Taxonomies}

	service.cacheWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       context.Background(),
		Logger:    service.logger,
		Operation: "cache commit",
		Fn: func() error {
			timer := timeutil.StartPhase("Cache commit (incremental)")
			if err := service.cache.BatchCommit([]*cache.ContentMeta{newMeta}, map[string]*cache.SearchRecord{ContentID: newSearch}, map[string]*models.Dependencies{ContentID: newDep}); err != nil {
				service.logger.Error("Failed to commit post to cache", "path", options.relPath, "error", err)
			}
			timer.Stop()
			return nil
		},
		Cleanup: service.cacheWg.Done,
	})
}

func (service *contentService) buildCacheMeta(options commitContentCacheOptions, contentID string) *cache.ContentMeta {
	cacheTOC := make([]models.TOCEntry, len(options.parseRes.TOC))
	for idx, tocEntry := range options.parseRes.TOC {
		cacheTOC[idx] = models.TOCEntry{ID: tocEntry.ID, Text: tocEntry.Text, Level: tocEntry.Level}
	}

	return &cache.ContentMeta{
		ContentID: contentID, Path: options.relPath, ModTime: options.info.ModTime().UnixNano(),
		ContentHash: options.parseRes.FrontmatterHash, BodyHash: hashing.GetBodyHash(nil),
		Title: options.item.Title, Date: options.item.DateObj, Taxonomies: options.item.Taxonomies,
		ReadingTime: options.item.ReadingTime, Description: options.item.Description,
		Link: options.item.Link, IsPinned: options.item.IsPinned, Weight: options.item.Weight,
		Section: options.item.Section,
		IsDraft: options.item.IsDraft, Meta: options.parseRes.Metadata, TOC: cacheTOC,
		SSRInputHashes: options.parseRes.SSRHashes,
		CardHash:       options.parseRes.FrontmatterHash,
		HasImages:      options.parseRes.HasImages,
	}
}

func (service *contentService) buildSearchRecord(parseRes *ParsedMarkdownResult) *cache.SearchRecord {
	return &cache.SearchRecord{
		Title:           parseRes.Item.Title,
		NormalizedTitle: strings.ToLower(parseRes.Item.Title),
		WordFreqs:       parseRes.WordFreqs,
		DocLen:          parseRes.DocLen,
		Content:         parseRes.PlainText,
		Taxonomies:      parseRes.SearchRecord.Taxonomies,
		NormalizedTaxs:  parseRes.SearchRecord.NormalizedTaxs,
		StemMap:         parseRes.StemMap,
		PositionalIndex: parseRes.PositionalIndex,
		ByteOffsets:     parseRes.ByteOffsets,
	}
}

func (service *contentService) handleSocialCard(parseRes *ParsedMarkdownResult, relPath, cardRelPath, cardDestPath string) {
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
