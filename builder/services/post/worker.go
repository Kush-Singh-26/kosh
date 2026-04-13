package post

import (
	"context"
	"strings"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (service *postService) loadCachedPost(relPath, htmlRelPath string, file models.ScannedFile, cachedMeta *models.PostMeta, useCache bool) (*ParsedMarkdownResult, string, []string, bool) {
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

func (service *postService) loadSourceIfNeeded(file models.ScannedFile, useCache bool) ([]byte, error) {
	if useCache && !service.cfg.Features.UseRawMarkdown {
		return nil, nil
	}
	if file.SourceLoader == nil {
		return nil, nil
	}
	return file.SourceLoader()
}

func (service *postService) parseIfNeeded(ctx context.Context, file models.ScannedFile, cachedMeta *models.PostMeta, htmlRelPath string, sourceBytes []byte, useCache bool) (*ParsedMarkdownResult, string, []string, bool, error) {
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
		CleanHtmlRelPath:     htmlRelPath,
		HtmlRelPath:          htmlRelPath,
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

func (service *postService) parseWorkerTaskLocal(file models.ScannedFile, workerContext WorkerContext, local *workerLocalState) {
	path := file.Path
	relativePath := file.RelPath

	htmlRelativePath, _, destinationPath := navigation.ComputePathVars(service.cfg.OutputDir, relativePath)

	// 1. Check Cache
	cachedMeta, useCache := service.checkCache(relativePath, file, workerContext.ShouldForce)

	parseResult, htmlContent, finalSSRHashes, useCache := service.loadCachedPost(relativePath, htmlRelativePath, file, cachedMeta, useCache)

	// 3. Full Parse if needed
	sourceBytes, err := service.loadSourceIfNeeded(file, useCache)
	if err != nil {
		service.logger.Error("Failed to load source", "path", path, "error", err)
		local.errs = append(local.errs, err)
		return
	}

	if !useCache {
		var parseErr error
		parseResult, htmlContent, finalSSRHashes, useCache, parseErr = service.parseIfNeeded(workerContext.Ctx, file, cachedMeta, htmlRelativePath, sourceBytes, useCache)
		if parseErr != nil {
			service.logger.Error("Failed to parse markdown", "path", path, "error", parseErr)
			local.errs = append(local.errs, parseErr)
			return
		}
		local.anyChanged = true
	}

	post := parseResult.Post
	if post.IsDraft && !service.cfg.ShouldIncludeDrafts {
		return
	}

	// 4. Social Card
	service.queueSocialCard(SocialCardOptions{
		RelativePath:       relativePath,
		Result:             parseResult,
		HtmlRelativePath:   htmlRelativePath,
		ForceSocialRebuild: workerContext.ForceSocialRebuild,
		CardPool:           workerContext.CardPool,
	})

	// 5. Aggregate and stream
	service.aggregateLocal(AggregateContext{
		ScannedFile:      file,
		Result:           parseResult,
		Post:             post,
		HtmlContent:      htmlContent,
		DestinationPath:  destinationPath,
		RelativePath:     relativePath,
		HtmlRelativePath: htmlRelativePath,
		SSRHashes:        finalSSRHashes,
		UseCache:         useCache,
		WorkerContext:    workerContext,
		Local:            local,
		SourceBytes:      sourceBytes,
	})
	if service.metrics != nil {
		service.metrics.IncrementPostsProcessed()
	}
}

func (service *postService) aggregateLocal(aggregateContext AggregateContext) {
	file := aggregateContext.ScannedFile
	renderResult := aggregateContext.Result
	post := aggregateContext.Post
	htmlContent := aggregateContext.HtmlContent
	destinationPath := aggregateContext.DestinationPath
	relativePath := aggregateContext.RelativePath
	htmlRelativePath := aggregateContext.HtmlRelativePath
	ssrHashes := aggregateContext.SSRHashes
	useCache := aggregateContext.UseCache
	workerContext := aggregateContext.WorkerContext
	local := aggregateContext.Local
	sourceBytes := aggregateContext.SourceBytes

	searchRecord := renderResult.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)

	localIndex := len(local.indexedPosts)
	local.indexedPosts = append(local.indexedPosts, models.IndexedPost{
		Record: searchRecord, WordFreqs: renderResult.WordFreqs, DocLen: renderResult.DocLen,
		StemMap: renderResult.StemMap, PositionalIndex: renderResult.PositionalIndex, ByteOffsets: renderResult.ByteOffsets,
	})

	if !useCache && service.cache != nil {
		newSearch := &models.SearchRecord{
			Title: post.Title, NormalizedTitle: renderResult.SearchRecord.NormalizedTitle,
			Content: renderResult.SearchRecord.Content, NormalizedTags: renderResult.SearchRecord.NormalizedTags,
		}
		postID := cache.GeneratePostID("", relativePath)
		local.newSearchRecords[postID] = newSearch

		if service.cfg.Features.Generators.IsSearchEnabled {
			local.searchTasks = append(local.searchTasks, deferredSearchTask{
				record: searchRecord, plainText: renderResult.PlainText, localIndex: localIndex, cached: newSearch,
			})
		}
	}

	local.allPosts = append(local.allPosts, post)
	if post.IsPinned {
		local.pinnedPosts = append(local.pinnedPosts, post)
	}
	for _, tag := range post.Tags {
		local.tagEntries = append(local.tagEntries, tagEntry{
			tag: strings.ToLower(strings.TrimSpace(tag)), post: post,
		})
	}

	// Stream to renderer // No locks needed for channel write
	workerContext.RenderChan <- renderTask{
		parseResult:      renderResult,
		file:             file,
		htmlContent:      htmlContent,
		destinationPath:  destinationPath,
		relativePath:     relativePath,
		htmlRelativePath: htmlRelativePath,
		source:           sourceBytes,
	}

	if !useCache && service.cache != nil {
		postID := cache.GeneratePostID("", relativePath)
		newMeta := &models.PostMeta{
			PostID: postID, Path: relativePath, ModTime: file.Info.ModTime().UnixNano(),
			ContentHash: renderResult.FrontmatterHash, BodyHash: file.BodyHash, Title: post.Title, Date: post.DateObj,
			WordCount: int(file.Info.Size()), // Use WordCount as size for quick comparison
			Tags:      post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, IsPinned: post.IsPinned, Weight: post.Weight, IsDraft: post.IsDraft,
			Meta: renderResult.Metadata, TOC: renderResult.TOC, SSRInputHashes: ssrHashes,
			CardHash: renderResult.FrontmatterHash, HasImages: renderResult.HasImages, MathExpressions: renderResult.MathExpressions,
		}
		if err := service.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
			service.logger.Warn("Failed to store HTML for post", "path", relativePath, "error", err)
		}
		local.newPostsMeta = append(local.newPostsMeta, newMeta)
		local.newDependencies[postID] = &models.Dependencies{Tags: post.Tags}
	}
}

func (service *postService) mergeWorkerStates(locals []*workerLocalState, workerContext WorkerContext) {
	processContext := workerContext.ProcessContext
	for _, local := range locals {
		if local.anyChanged {
			processContext.anyPostChanged.Store(true)
		}
		baseIndex := len(processContext.indexedPosts)
		processContext.allPosts = append(processContext.allPosts, local.allPosts...)
		processContext.pinnedPosts = append(processContext.pinnedPosts, local.pinnedPosts...)
		processContext.indexedPosts = append(processContext.indexedPosts, local.indexedPosts...)
		processContext.newPostsMeta = append(processContext.newPostsMeta, local.newPostsMeta...)
		for k, v := range local.newSearchRecords {
			processContext.newSearchRecords[k] = v
		}
		for k, v := range local.newDependencies {
			processContext.newDependencies[k] = v
		}
		for _, entry := range local.tagEntries {
			processContext.tagMap[entry.tag] = append(processContext.tagMap[entry.tag], entry.post)
		}
		processContext.errs = append(processContext.errs, local.errs...)

		for _, task := range local.searchTasks {
			globalIndex := baseIndex + task.localIndex
			workerContext.SearchPool.Submit(searchTask{
				record:    task.record,
				plainText: task.plainText,
				indexed:   &processContext.indexedPosts[globalIndex],
				cached:    task.cached,
			})
		}
	}
}
