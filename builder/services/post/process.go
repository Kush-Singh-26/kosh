package post

import (
	"context"
	"html/template"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

const (
	navReadyBuffer       = 1
	renderChanMultiplier = 2
	indexedPostsCap      = 50
	collectedFilesBuffer = 1024
)

// Process processes a set of files with a buffered channel.
func (service *postService) Process(opts ProcessOptions) (*ContentResult, error) {
	fileChan := make(chan models.ScannedResource, len(opts.Files))
	for _, file := range opts.Files {
		fileChan <- file
	}
	close(fileChan)
	opts.FileChan = fileChan
	return service.ProcessStreaming(opts)
}

func (service *postService) startCardPool(ctx context.Context, numWorkers int) *async.WorkerPool[socialCardTask] {
	pool := async.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) error {
		service.generateSocialCard(task)
		return nil
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskSocialCard)
	pool.Start()
	return pool
}

func (service *postService) startSearchPool(ctx context.Context, numWorkers int) *async.WorkerPool[searchTask] {
	pool := async.NewWorkerPool(ctx, numWorkers, func(task searchTask) error {
		wordFreqs, docLen, stemMap, posIndex, byteOffsets := tokenizeSearchData(task.record, task.plainText)

		task.indexed.WordFreqs = wordFreqs
		task.indexed.DocLen = docLen
		task.indexed.StemMap = stemMap
		task.indexed.PositionalIndex = posIndex
		task.indexed.ByteOffsets = byteOffsets

		if task.cached != nil {
			task.cached.BM25Data = wordFreqs
			task.cached.DocLen = docLen
			task.cached.StemMap = stemMap
			task.cached.PositionalIndex = posIndex
			task.cached.ByteOffsets = byteOffsets
		}

		if task.SearchIngestor != nil {
			task.SearchIngestor.Add(*task.indexed)
		}
		return nil
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskMarkdown)
	pool.Start()
	return pool
}

func startNavCollector(ctx context.Context, logger *slog.Logger, prepare func([]models.ScannedResource) navInfo) (chan models.ScannedResource, chan navInfo, *sync.WaitGroup) {
	collectedFilesChan := make(chan models.ScannedResource, collectedFilesBuffer)
	navReady := make(chan navInfo, navReadyBuffer)
	var allFiles []models.ScannedResource
	var collectWg sync.WaitGroup

	collectWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "collect scan results",
		Fn: func() error {
			for file := range collectedFilesChan {
				allFiles = append(allFiles, file)
			}
			navReady <- prepare(allFiles)
			return nil
		},
		Cleanup: collectWg.Done,
	})

	return collectedFilesChan, navReady, &collectWg
}

type renderTaskCollector struct {
	renderChan      chan renderTask
	renderTasks     []renderTask
	renderTasksMu   sync.Mutex
	renderTasksDone chan struct{}
}

func startRenderTaskCollector(ctx context.Context, logger *slog.Logger, numWorkers int) *renderTaskCollector {
	collector := &renderTaskCollector{
		renderChan:      make(chan renderTask, numWorkers*renderChanMultiplier),
		renderTasksDone: make(chan struct{}),
	}
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "collect render tasks",
		Fn: func() error {
			for renTask := range collector.renderChan {
				collector.renderTasksMu.Lock()
				collector.renderTasks = append(collector.renderTasks, renTask)
				collector.renderTasksMu.Unlock()
			}
			return nil
		},
		Cleanup: func() { close(collector.renderTasksDone) },
	})
	return collector
}

func waitForRenderTasks(collector *renderTaskCollector) []renderTask {
	<-collector.renderTasksDone
	collector.renderTasksMu.Lock()
	defer collector.renderTasksMu.Unlock()
	return append([]renderTask(nil), collector.renderTasks...)
}

func finalizePostProcessing(processCtx *postProcessContext) {
	timeutil.SortPosts(processCtx.allPosts)
	timeutil.SortPosts(processCtx.pinnedPosts)
	for _, termMap := range processCtx.taxonomyMap {
		for _, posts := range termMap {
			timeutil.SortPosts(posts)
		}
	}
}

// ProcessStreaming processes posts using streaming parse and render phases.
func (service *postService) ProcessStreaming(opts ProcessOptions) (*ContentResult, error) {
	ctx := opts.Ctx
	searchIngestor := opts.SearchIngestor
	shouldForce := opts.ShouldForce
	forceSocialRebuild := opts.ForceSocialRebuild
	fileChan := opts.FileChan

	numWorkers := models.GetDefaultWorkerCount()

	cardPool := service.startCardPool(ctx, numWorkers)
	defer func() { buildctx.IgnoreError(cardPool.Stop(), "stop card pool") }()

	// Search Indexing Pool (Background)
	searchPool := service.startSearchPool(ctx, numWorkers)
	defer func() { buildctx.IgnoreError(searchPool.Stop(), "stop search pool") }()

	// Create a channel for navInfo to be sent once scanner finishes
	totalFiles := len(opts.Files)
	processCtx := &postProcessContext{
		allPosts:         make([]models.PostMetadata, 0, totalFiles),
		pinnedPosts:      make([]models.PostMetadata, 0, totalFiles/4),
		taxonomyMap:      make(map[string]map[string][]models.PostMetadata),
		newSearchRecords: make(map[string]*models.SearchRecord),
		newDependencies:  make(map[string]*models.Dependencies),
		indexedPosts:     make([]models.IndexedPost, 0, totalFiles),
		newPostsMeta:     make([]*models.PostMeta, 0, totalFiles),
	}

	// Internal channel to collect all files for navigation calculation
	logger := service.logger
	if logger == nil {
		logger = slog.Default()
	}
	collectedFilesChan, navReady, collectWg := startNavCollector(ctx, logger, service.prepareNavigationInfo)

	// Start render task collector goroutine to avoid blocking parse workers
	renderCollector := startRenderTaskCollector(ctx, logger, numWorkers)

	// renderWg is kept for context-based cleanup if needed, though runStreamingRenderPhase is synchronous
	var renderWg sync.WaitGroup
	renderWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "render phase sync",
		Fn: func() error {
			// This goroutine will wait for renderChan to close and tasks to be collected elsewhere
			return nil
		},
		Cleanup: renderWg.Done,
	})

	workerCtx := WorkerContext{
		Ctx: ctx, ProcessContext: processCtx, CardPool: cardPool, SearchPool: searchPool,
		SearchIngestor: searchIngestor,
		RenderChan:     renderCollector.renderChan, ShouldForce: shouldForce, ForceSocialRebuild: forceSocialRebuild,
	}

	// Stream from original channel to both parse workers and our navigation collector
	err := service.runStreamingParsePhase(numWorkers, fileChan, collectedFilesChan, workerCtx)
	close(collectedFilesChan)
	collectWg.Wait() // Wait for scanner walk to finish and navReady to be filled

	// Once scanner is done and navReady is filled, signal the render phase
	close(renderCollector.renderChan) // No more render tasks will be produced
	renderTasks := waitForRenderTasks(renderCollector)

	// Global SSR render pass (Math and D2)
	service.renderSSRGlobal(ctx, renderTasks)

	// Now start the render pool with complete navInfo
	nav := <-navReady
	service.runStreamingRenderPhase(ctx, numWorkers, nav, renderTasks)

	renderWg.Wait() // Reserved for backward compatibility if needed, but runStreamingRenderPhase is now synchronous here

	if err != nil {
		return nil, err
	}

	// Wait for search indexing to complete before returning results
	if err := searchPool.Stop(); err != nil {
		service.logger.Error("Failed to stop search pool", "error", err)
	}

	service.finalizeBuild(processCtx)

	// Sort allPosts to ensure deterministic ordering across builds
	finalizePostProcessing(processCtx)

	return &ContentResult{
		AllPosts: processCtx.allPosts, PinnedPosts: processCtx.pinnedPosts, TaxonomyMap: processCtx.taxonomyMap,
		IndexedPosts: processCtx.indexedPosts, AnyPostChanged: processCtx.anyPostChanged.Load(), Has404: false,
	}, nil
}

// GetMetadataContext retrieves the full site metadata context from the post cache.
func (service *postService) GetMetadataContext(ctx context.Context) (*ContentContext, error) {
	if service.cache == nil {
		return &ContentContext{}, nil
	}

	ids, err := service.cache.ListAllPosts()
	if err != nil {
		return nil, err
	}

	metas, err := service.cache.GetPostsByIDs(ids)
	if err != nil {
		return nil, err
	}

	var allPosts []models.PostMetadata
	pinnedPosts := make([]models.PostMetadata, 0)
	taxonomyMap := make(map[string]map[string][]models.PostMetadata)

	for _, meta := range metas {
		if meta.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		postMeta := models.PostMetadata{
			Title: meta.Title, Link: meta.Link, Description: meta.Description,
			IsPinned: meta.IsPinned, Weight: meta.Weight,
			ReadingTime: meta.ReadingTime, DateObj: meta.Date,
			IsDraft:    meta.IsDraft,
			Taxonomies: meta.Taxonomies,
		}

		// Taxonomy extraction logic removed: directly utilizing meta.Taxonomies from cache.

		allPosts = append(allPosts, postMeta)
		if postMeta.IsPinned {
			pinnedPosts = append(pinnedPosts, postMeta)
		}

		// Aggregate into taxonomyMap
		for taxKey, terms := range postMeta.Taxonomies {
			if taxonomyMap[taxKey] == nil {
				taxonomyMap[taxKey] = make(map[string][]models.PostMetadata)
			}
			for _, term := range terms {
				normTerm := strings.ToLower(strings.TrimSpace(term))
				taxonomyMap[taxKey][normTerm] = append(taxonomyMap[taxKey][normTerm], postMeta)
			}
		}
	}

	timeutil.SortPosts(allPosts)
	timeutil.SortPosts(pinnedPosts)
	for _, termMap := range taxonomyMap {
		for _, posts := range termMap {
			timeutil.SortPosts(posts)
		}
	}

	return &ContentContext{
		AllPosts:    allPosts,
		PinnedPosts: pinnedPosts,
		TaxonomyMap: taxonomyMap,
	}, nil
}

func (service *postService) runStreamingParsePhase(numWorkers int, fileChan <-chan models.ScannedResource, collector chan<- models.ScannedResource, workerCtx WorkerContext) error {
	service.logger.Info("Processing posts (streaming mode)")
	timer := timeutil.StartPhase("Process posts (stream)")
	defer timer.Stop()

	locals := make([]*workerLocalState, numWorkers)
	expectedPerWorker := (len(fileChan) / numWorkers) + 1
	for i := range locals {
		locals[i] = &workerLocalState{
			allPosts:         make([]models.PostMetadata, 0, expectedPerWorker),
			pinnedPosts:      make([]models.PostMetadata, 0, expectedPerWorker/4),
			indexedPosts:     make([]models.IndexedPost, 0, expectedPerWorker),
			searchTasks:      make([]deferredSearchTask, 0, expectedPerWorker),
			newPostsMeta:     make([]*models.PostMeta, 0, expectedPerWorker),
			newSearchRecords: make(map[string]*models.SearchRecord),
			newDependencies:  make(map[string]*models.Dependencies),
		}
	}
	var workerIdx atomic.Int32

	parsePool := async.NewWorkerPool(workerCtx.Ctx, numWorkers, func(file models.ScannedResource) error {
		idx := int(workerIdx.Add(1)-1) % numWorkers
		service.parseWorkerTaskLocal(file, workerCtx, locals[idx])
		return nil
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskMarkdown)

	parsePool.Start()
	for file := range fileChan {
		if collector != nil {
			collector <- file
		}
		parsePool.Submit(file)
	}
	err := parsePool.Stop()

	service.mergeWorkerStates(locals, workerCtx)

	return err
}

func (service *postService) runStreamingRenderPhase(ctx context.Context, numWorkers int, nav navInfo, tasks []renderTask) {
	processed := atomic.Int32{}
	totalFiles := len(tasks)

	// Build a map to update post metadata with finalized HTML for RSS
	postMap := make(map[string]*models.PostMetadata)
	if service.cfg.Features.Generators.IsRSSEnabled {
		for i := range nav.allPosts {
			postMap[nav.allPosts[i].Link] = &nav.allPosts[i]
		}
	}

	renderPool := async.NewWorkerPool(ctx, numWorkers, func(renderTaskInstance renderTask) error {
		post := renderTaskInstance.parseResult.Post
		htmlContent := renderTaskInstance.htmlContent
		relPrefix := fspkg.GetRelativePrefix(renderTaskInstance.htmlRelativePath)
		htmlContent = rewriteStaticAssetPaths(htmlContent, relPrefix)

		_, _, cardImageURL := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, renderTaskInstance.htmlRelativePath)
		var prev, next *models.NavPage
		if position, ok := nav.postPos[renderTaskInstance.file.Link]; ok {
			allPosts := nav.allPosts
			if position > 0 {
				prev = &models.NavPage{Title: allPosts[position-1].Title, Link: allPosts[position-1].Link}
			}
			if position < len(allPosts)-1 {
				next = &models.NavPage{Title: allPosts[position+1].Title, Link: allPosts[position+1].Link}
			}
		}

		postsIndexURL := navigation.ResolveSectionIndex(renderTaskInstance.htmlRelativePath)

		// Determine node type and layout
		layoutVal := strings.ToLower(renderTaskInstance.file.Layout)
		if layoutVal == "" {
			// Fallback to frontmatter if not set by Data Cascade
			if l, ok := renderTaskInstance.parseResult.Metadata["layout"].(string); ok {
				layoutVal = strings.ToLower(l)
			}
		}

		pageContext := models.ContextPosts
		if layoutVal == "home" {
			pageContext = models.ContextHome
		}

		if err := service.renderer.RenderPage(renderTaskInstance.destinationPath, models.PageData{
			Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
			Meta: renderTaskInstance.parseResult.Metadata, BaseURL: service.cfg.BaseURL, BuildVersion: service.cfg.BuildVersion,
			TabTitle: post.Title + " | " + service.cfg.Title, Permalink: renderTaskInstance.file.Link, Image: cardImageURL,
			TOC: renderTaskInstance.parseResult.TOC, Config: service.cfg, ReadingTime: post.ReadingTime,
			Taxonomies: nav.taxonomies,
			PrevPage:   prev, NextPage: next, RelativePrefix: relPrefix,
			HasImages: renderTaskInstance.parseResult.HasImages, Context: pageContext,
			ContentPrefix: service.cfg.ContentPrefix,
			PostsIndexURL: postsIndexURL,
			SiteData:      service.cfg.SiteData,
			JSONLD:        service.generateJSONLD(post, cardImageURL),
			Section:       post.Section,
			IsCleanBuild:  service.ctx.IsCleanBuild,
		}); err != nil {
			return err
		}

		if service.cfg.Features.UseRawMarkdown {
			mdDestPath := renderTaskInstance.destinationPath[:len(renderTaskInstance.destinationPath)-len(filepath.Ext(renderTaskInstance.destinationPath))] + ".md"
			if err := service.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
				service.logger.Warn("Failed to create directory for raw markdown", "dir", filepath.Dir(mdDestPath), "error", err)
			}
			if err := service.sink.WriteFile(mdDestPath, renderTaskInstance.source); err == nil {
				service.renderer.RegisterFile(mdDestPath)
			}
		}

		if service.reporter != nil {
			curr := int(processed.Add(1))
			service.reporter.UpdateProgress(ui.PhasePosts, curr, totalFiles, renderTaskInstance.relativePath)
		}

		return nil
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskMarkdown)

	renderPool.Start()
	for _, renderTaskInstance := range tasks {
		renderPool.Submit(renderTaskInstance)
	}
	if err := renderPool.Stop(); err != nil {
		service.logger.Error("Failed to stop render pool", "error", err)
	}
}
