package post

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (service *postService) loadCachedPost(_ string, htmlRelPath string, _ models.ScannedResource, cachedMeta *models.PostMeta, useCache bool) (*ParsedMarkdownResult, string, []string, bool) {
	var parseRes *ParsedMarkdownResult
	var htmlContent string
	var finalSSRHashes []string

	if useCache {
		parseRes, htmlContent, useCache = service.loadFromCache(cachedMeta, htmlRelPath)
		if useCache {
			finalSSRHashes = cachedMeta.SSRInputHashes
			if service.metrics != nil {
				service.metrics.IncrementCacheHit()
			}
		}
	}

	if useCache && htmlContent != "" && len(parseRes.MathExpressions) > 0 {
		var mathOk bool
		htmlContent, mathOk = service.processCachedMath(htmlContent, parseRes.MathExpressions)
		useCache = useCache && mathOk
	}

	if useCache && htmlContent != "" && len(parseRes.D2Expressions) > 0 {
		var d2Ok bool
		htmlContent, d2Ok = service.processCachedD2(htmlContent, parseRes.D2Expressions)
		useCache = useCache && d2Ok
	}

	return parseRes, htmlContent, finalSSRHashes, useCache
}

func (service *postService) loadSourceIfNeeded(file models.ScannedResource, useCache bool) ([]byte, error) {
	if useCache && !service.cfg.Features.UseRawMarkdown {
		return nil, nil
	}
	if file.SourceLoader == nil {
		return nil, nil
	}
	return file.SourceLoader()
}

func (service *postService) parseIfNeeded(_ context.Context, file models.ScannedResource, cachedMeta *models.PostMeta, htmlRelPath string, sourceBytes []byte, useCache bool) (*ParsedMarkdownResult, string, []string, bool, error) {
	if useCache {
		return nil, "", nil, true, nil
	}

	if service.metrics != nil {
		service.metrics.IncrementCacheMiss()
	}

	readingTime := file.ReadingTime
	if cachedMeta != nil && cachedMeta.BodyHash == file.BodyHash && cachedMeta.ReadingTime > 0 {
		readingTime = cachedMeta.ReadingTime
	}

	// Apply shortcode processing if processor is available
	if service.shortcodes != nil && len(sourceBytes) > 0 {
		processed, err := service.shortcodes.Process(sourceBytes)
		if err == nil {
			sourceBytes = processed
		} else {
			service.logger.Warn("Shortcode processing failed", "path", file.Path, "error", err)
		}
	}

	parseRes, err := ParseMarkdown(ParseOptions{
		Path:                 file.Path,
		RelPath:              file.RelPath,
		Source:               sourceBytes,
		Info:                 file.Info,
		Renderer:             service.renderer,
		NativeRenderer:       service.nativeRenderer,
		MdPool:               service.mdPool,
		DiagramAdapter:       service.diagramAdapter,
		Metrics:              service.metrics,
		Cfg:                  service.cfg,
		CleanHTMLRelPath:     htmlRelPath,
		HTMLRelPath:          htmlRelPath,
		KnownFrontmatterHash: file.FrontmatterHash,
		KnownReadingTime:     readingTime,
		BodyOffset:           file.BodyOffset,
		PreParsedMeta:        file.PreParsedMeta,
	})
	if err != nil {
		return nil, "", nil, false, err
	}

	if parseRes.Post.Title == "" {
		parseRes.Post.Title = file.Title
	}

	htmlContent := parseRes.HTMLContent
	finalSSRHashes := parseRes.SSRHashes
	return parseRes, htmlContent, finalSSRHashes, false, nil
}

func (service *postService) calculateSection(relativePath string) string {
	cleanRel := filepath.ToSlash(relativePath)
	cleanRel = strings.TrimPrefix(cleanRel, "./")
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	parts := strings.Split(cleanRel, "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func (service *postService) parseWorkerTaskLocal(file models.ScannedResource, workerContext WorkerContext, local *workerLocalState) {
	relativePath := file.RelPath
	htmlRelativePath, _, destinationPath := navigation.ComputePathVars(service.cfg.OutputDir, relativePath)

	cachedMeta, useCache := service.checkCache(relativePath, file, workerContext.ShouldForce)
	parseResult, htmlContent, finalSSRHashes, useCache := service.loadCachedPost(relativePath, htmlRelativePath, file, cachedMeta, useCache)

	sourceBytes, err := service.loadSourceIfNeeded(file, useCache)
	if err != nil {
		service.logger.Error("Failed to load source", "path", file.Path, "error", err)
		local.errs = append(local.errs, err)
		return
	}

	if !useCache {
		var parseErr error
		parseResult, htmlContent, finalSSRHashes, useCache, parseErr = service.parseIfNeeded(workerContext.Ctx, file, cachedMeta, htmlRelativePath, sourceBytes, useCache)
		if parseErr != nil {
			service.logger.Error("Failed to parse markdown", "path", file.Path, "error", parseErr)
			local.errs = append(local.errs, parseErr)
			return
		}
		local.anyChanged = true
	}

	post := parseResult.Post
	if post.IsDraft && !service.cfg.ShouldIncludeDrafts {
		return
	}

	section := service.calculateSection(relativePath)
	post.Section = section
	parseResult.Post.Section = section

	service.queueSocialCard(SocialCardOptions{
		RelativePath:       relativePath,
		Result:             parseResult,
		HTMLRelativePath:   htmlRelativePath,
		ForceSocialRebuild: workerContext.ForceSocialRebuild,
		CardPool:           workerContext.CardPool,
	})

	service.aggregateLocal(AggregateContext{
		ScannedFile: file, Result: parseResult, Post: post, HTMLContent: htmlContent,
		DestinationPath: destinationPath, RelativePath: relativePath, HTMLRelativePath: htmlRelativePath,
		SSRHashes: finalSSRHashes, UseCache: useCache, WorkerContext: workerContext, Local: local, SourceBytes: sourceBytes,
	})
	if service.metrics != nil {
		service.metrics.IncrementPostsProcessed()
	}
}

func (service *postService) prepareIndexedPost(aggregateContext AggregateContext) models.IndexedPost {
	searchRecord := aggregateContext.Result.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)

	return models.IndexedPost{
		Record: searchRecord, WordFreqs: aggregateContext.Result.WordFreqs, DocLen: aggregateContext.Result.DocLen,
		StemMap: aggregateContext.Result.StemMap, PositionalIndex: aggregateContext.Result.PositionalIndex, ByteOffsets: aggregateContext.Result.ByteOffsets,
	}
}

func (service *postService) handleSearchTasks(aggregateContext AggregateContext, indexed models.IndexedPost, localIndex int) {
	if aggregateContext.UseCache && aggregateContext.WorkerContext.SearchIngestor != nil {
		aggregateContext.WorkerContext.SearchIngestor.Add(indexed)
	}

	if !aggregateContext.UseCache && service.cache != nil {
		newSearch := &models.SearchRecord{
			Title: aggregateContext.Post.Title, NormalizedTitle: aggregateContext.Result.SearchRecord.NormalizedTitle,
			Content:        aggregateContext.Result.SearchRecord.Content,
			Taxonomies:     aggregateContext.Result.SearchRecord.Taxonomies,
			NormalizedTaxs: aggregateContext.Result.SearchRecord.NormalizedTaxs,
		}
		postID := cache.GeneratePostID("", aggregateContext.RelativePath)
		aggregateContext.Local.newSearchRecords[postID] = newSearch

		if service.cfg.Features.Generators.IsSearchEnabled {
			aggregateContext.Local.searchTasks = append(aggregateContext.Local.searchTasks, deferredSearchTask{
				record: indexed.Record, plainText: aggregateContext.Result.PlainText, localIndex: localIndex, cached: newSearch,
			})
		}
	}
}

func (service *postService) storeCacheMetadata(aggregateContext AggregateContext) {
	if aggregateContext.UseCache || service.cache == nil {
		return
	}

	postID := cache.GeneratePostID("", aggregateContext.RelativePath)
	newMeta := &models.PostMeta{
		PostID: postID, Path: aggregateContext.RelativePath, ModTime: aggregateContext.ScannedFile.Info.ModTime().UnixNano(),
		ContentHash: aggregateContext.Result.FrontmatterHash, BodyHash: aggregateContext.ScannedFile.BodyHash,
		Title: aggregateContext.Post.Title, Date: aggregateContext.Post.DateObj,
		WordCount:  int(aggregateContext.ScannedFile.Info.Size()),
		Taxonomies: aggregateContext.Post.Taxonomies, ReadingTime: aggregateContext.Post.ReadingTime,
		Description: aggregateContext.Post.Description, Link: aggregateContext.Post.Link,
		IsPinned: aggregateContext.Post.IsPinned, Weight: aggregateContext.Post.Weight,
		IsDraft: aggregateContext.Post.IsDraft, Meta: aggregateContext.Result.Metadata,
		TOC: aggregateContext.Result.TOC, SSRInputHashes: aggregateContext.SSRHashes,
		CardHash: aggregateContext.Result.FrontmatterHash, HasImages: aggregateContext.Result.HasImages,
		MathExpressions: aggregateContext.Result.MathExpressions,
	}

	if err := service.cache.StoreHTMLForPost(newMeta, []byte(aggregateContext.HTMLContent)); err != nil {
		service.logger.Warn("Failed to store HTML for post", "path", aggregateContext.RelativePath, "error", err)
	}

	aggregateContext.Local.newPostsMeta = append(aggregateContext.Local.newPostsMeta, newMeta)
	aggregateContext.Local.newDependencies[postID] = &models.Dependencies{Taxonomies: aggregateContext.Post.Taxonomies}
}

func (service *postService) aggregateLocal(aggregateContext AggregateContext) {
	local := aggregateContext.Local

	indexed := service.prepareIndexedPost(aggregateContext)
	localIndex := len(local.indexedPosts)
	local.indexedPosts = append(local.indexedPosts, indexed)

	service.handleSearchTasks(aggregateContext, indexed, localIndex)

	local.allPosts = append(local.allPosts, aggregateContext.Post)
	if aggregateContext.Post.IsPinned {
		local.pinnedItems = append(local.pinnedItems, aggregateContext.Post)
	}

	for taxKey, terms := range aggregateContext.Post.Taxonomies {
		for _, term := range terms {
			local.taxonomyEntries = append(local.taxonomyEntries, taxonomyEntry{
				taxonomy: taxKey,
				term:     strings.ToLower(strings.TrimSpace(term)),
				post:     aggregateContext.Post,
			})
		}
	}

	aggregateContext.WorkerContext.RenderChan <- renderTask{
		parseResult:      aggregateContext.Result,
		file:             aggregateContext.ScannedFile,
		htmlContent:      aggregateContext.HTMLContent,
		destinationPath:  aggregateContext.DestinationPath,
		relativePath:     aggregateContext.RelativePath,
		htmlRelativePath: aggregateContext.HTMLRelativePath,
		source:           aggregateContext.SourceBytes,
	}

	service.storeCacheMetadata(aggregateContext)
}

func (service *postService) mergeWorkerStates(locals []*workerLocalState, workerContext WorkerContext) {
	processContext := workerContext.ProcessContext
	baseIndices := make([]int, len(locals))

	// Phase 1: Aggregate all metadata and stabilize the indexedPosts slice.
	// We must finish all appends to indexedPosts before handing out pointers to its elements
	// to avoid data races during slice reallocation (growslice).
	for i, local := range locals {
		if local.anyChanged {
			processContext.anyPostChanged.Store(true)
		}
		baseIndices[i] = len(processContext.indexedPosts)
		processContext.allPosts = append(processContext.allPosts, local.allPosts...)
		processContext.pinnedItems = append(processContext.pinnedItems, local.pinnedItems...)
		processContext.indexedPosts = append(processContext.indexedPosts, local.indexedPosts...)
		processContext.newPostsMeta = append(processContext.newPostsMeta, local.newPostsMeta...)
		for k, v := range local.newSearchRecords {
			processContext.newSearchRecords[k] = v
		}
		for k, v := range local.newDependencies {
			processContext.newDependencies[k] = v
		}
		for _, entry := range local.taxonomyEntries {
			if processContext.taxonomyMap[entry.taxonomy] == nil {
				processContext.taxonomyMap[entry.taxonomy] = make(map[string][]models.PostMetadata)
			}
			processContext.taxonomyMap[entry.taxonomy][entry.term] = append(processContext.taxonomyMap[entry.taxonomy][entry.term], entry.post)
		}
		processContext.errs = append(processContext.errs, local.errs...)
	}

	// Phase 2: Submit search tasks using stable pointers into the global slice.
	for i, local := range locals {
		baseIndex := baseIndices[i]
		for _, task := range local.searchTasks {
			globalIndex := baseIndex + task.localIndex
			workerContext.SearchPool.Submit(searchTask{
				record:         task.record,
				plainText:      task.plainText,
				indexed:        &processContext.indexedPosts[globalIndex],
				cached:         task.cached,
				SearchIngestor: workerContext.SearchIngestor,
			})
		}
	}
}
