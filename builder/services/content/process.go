package content

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/generators"
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
	indexedItemsCap      = 50
	collectedFilesBuffer = 1024
)

// Process processes a set of files with a buffered channel.
func (service *contentService) Process(opts ProcessOptions) (*Result, error) {
	fileChan := make(chan models.ScannedResource, len(opts.Files))
	for _, file := range opts.Files {
		fileChan <- file
	}
	close(fileChan)
	opts.FileChan = fileChan
	return service.ProcessStreaming(opts)
}

func (service *contentService) startCardPool(ctx context.Context, numWorkers int) *async.WorkerPool[socialCardTask] {
	pool := async.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) error {
		service.generateSocialCard(task)
		return nil
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskSocialCard)
	pool.Start()
	return pool
}

func (service *contentService) startSearchPool(ctx context.Context, numWorkers int) *async.WorkerPool[searchTask] {
	pool := async.NewWorkerPool(ctx, numWorkers, func(task searchTask) error {
		wordFreqs, docLen, stemMap, posIndex, byteOffsets := tokenizeSearchData(task.record, task.plainText)

		task.indexed.WordFreqs = wordFreqs
		task.indexed.DocLen = docLen
		task.indexed.StemMap = stemMap
		task.indexed.PositionalIndex = posIndex
		task.indexed.ByteOffsets = byteOffsets

		if task.cached != nil {
			task.cached.WordFreqs = wordFreqs
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

func finalizePostProcessing(processCtx *contentProcessContext) {
	timeutil.SortItems(processCtx.allItems)
	timeutil.SortItems(processCtx.pinnedItems)
	for _, termMap := range processCtx.taxonomyMap {
		for _, posts := range termMap {
			timeutil.SortItems(posts)
		}
	}
}

func (service *contentService) newcontentProcessContext(totalFiles int) *contentProcessContext {
	return &contentProcessContext{
		allItems:         make([]models.ContentMetadata, 0, totalFiles),
		pinnedItems:      make([]models.ContentMetadata, 0, totalFiles/4),
		taxonomyMap:      make(map[string]map[string][]models.ContentMetadata),
		newSearchRecords: make(map[string]*models.SearchRecord),
		newDependencies:  make(map[string]*models.Dependencies),
		indexedItems:     make([]models.IndexedContent, 0, totalFiles),
		newItemsMeta:     make([]*models.ContentMeta, 0, totalFiles),
	}
}

func (service *contentService) setupStreamingContext(ctx context.Context, numWorkers int) (*async.WorkerPool[socialCardTask], *async.WorkerPool[searchTask]) {
	cardPool := service.startCardPool(ctx, numWorkers)
	searchPool := service.startSearchPool(ctx, numWorkers)
	return cardPool, searchPool
}

func (service *contentService) getLogger() *slog.Logger {
	if service.logger != nil {
		return service.logger
	}
	return slog.Default()
}

// ProcessStreaming processes posts using streaming parse and render phases.
func (service *contentService) ProcessStreaming(opts ProcessOptions) (*Result, error) {
	ctx := opts.Ctx
	numWorkers := models.GetDefaultWorkerCount()

	cardPool, searchPool := service.setupStreamingContext(ctx, numWorkers)
	defer func() {
		if err := cardPool.Stop(); err != nil {
			service.logger.Error("Failed to stop card pool", "error", err)
		}
	}()
	defer func() {
		if err := searchPool.Stop(); err != nil {
			service.logger.Error("Failed to stop search pool", "error", err)
		}
	}()

	processCtx := service.newcontentProcessContext(len(opts.Files))
	logger := service.getLogger()

	collectedFilesChan, navReady, collectWg := startNavCollector(ctx, logger, service.prepareNavigationInfo)
	renderCollector := startRenderTaskCollector(ctx, logger, numWorkers)

	workerCtx := WorkerContext{
		Ctx: ctx, ProcessContext: processCtx, CardPool: cardPool, SearchPool: searchPool,
		SearchIngestor: opts.SearchIngestor,
		RenderChan:     renderCollector.renderChan,
		ShouldForce:    opts.ShouldForce,
		ForceSocialRebuild: opts.ForceSocialRebuild,
		ForceRerender:      opts.ForceRerender,
	}

	err := service.runStreamingParsePhase(numWorkers, opts.FileChan, collectedFilesChan, workerCtx)
	close(collectedFilesChan)
	collectWg.Wait()

	close(renderCollector.renderChan)
	renderTasks := waitForRenderTasks(renderCollector)

	renderedMath, renderedD2 := service.renderSSRGlobal(ctx, renderTasks)
	nav := <-navReady
	service.runStreamingRenderPhase(ctx, numWorkers, nav, renderTasks, renderedMath, renderedD2)

	if err != nil {
		return nil, err
	}

	service.finalizeBuild(processCtx)
	finalizePostProcessing(processCtx)

	return &Result{
		allItems: processCtx.allItems, PinnedItems: processCtx.pinnedItems, TaxonomyMap: processCtx.taxonomyMap,
		indexedItems: processCtx.indexedItems, anyItemChanged: processCtx.anyItemChanged.Load(), Has404: false,
	}, nil
}

// GetMetadataContext retrieves the full site metadata context from the post cache.
func (service *contentService) GetMetadataContext(_ context.Context) (*Context, error) {
	if service.cache == nil {
		return &Context{}, nil
	}

	ids, err := service.cache.ListAllItems()
	if err != nil {
		return nil, err
	}

	metas, err := service.cache.GetItemsByIDs(ids)
	if err != nil {
		return nil, err
	}

	var allItems []models.ContentMetadata
	pinnedItems := make([]models.ContentMetadata, 0)
	taxonomyMap := make(map[string]map[string][]models.ContentMetadata)

	for _, meta := range metas {
		if meta.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		ContentMeta := models.ContentMetadata{
			Title: meta.Title, Link: meta.Link, Description: meta.Description,
			IsPinned: meta.IsPinned, Weight: meta.Weight,
			ReadingTime: meta.ReadingTime, DateObj: meta.Date,
			IsDraft:    meta.IsDraft,
			Taxonomies: meta.Taxonomies,
		}

		// Taxonomy extraction logic removed: directly utilizing meta.Taxonomies from cache.

		allItems = append(allItems, ContentMeta)
		if ContentMeta.IsPinned {
			pinnedItems = append(pinnedItems, ContentMeta)
		}

		// Aggregate into taxonomyMap
		for taxKey, terms := range ContentMeta.Taxonomies {
			if taxonomyMap[taxKey] == nil {
				taxonomyMap[taxKey] = make(map[string][]models.ContentMetadata)
			}
			for _, term := range terms {
				normTerm := strings.ToLower(strings.TrimSpace(term))
				taxonomyMap[taxKey][normTerm] = append(taxonomyMap[taxKey][normTerm], ContentMeta)
			}
		}
	}

	timeutil.SortItems(allItems)
	timeutil.SortItems(pinnedItems)
	for _, termMap := range taxonomyMap {
		for _, posts := range termMap {
			timeutil.SortItems(posts)
		}
	}

	return &Context{
		AllItems:    allItems,
		PinnedItems: pinnedItems,
		TaxonomyMap: taxonomyMap,
	}, nil
}

func (service *contentService) runStreamingParsePhase(numWorkers int, fileChan <-chan models.ScannedResource, collector chan<- models.ScannedResource, workerCtx WorkerContext) error {
	service.logger.Info("Processing posts (streaming mode)")
	timer := timeutil.StartPhase("Process posts (stream)")
	defer timer.Stop()

	locals := make([]*workerLocalState, numWorkers)
	expectedPerWorker := (len(fileChan) / numWorkers) + 1
	for i := range locals {
		locals[i] = &workerLocalState{
			allItems:         make([]models.ContentMetadata, 0, expectedPerWorker),
			pinnedItems:      make([]models.ContentMetadata, 0, expectedPerWorker/4),
			indexedItems:     make([]models.IndexedContent, 0, expectedPerWorker),
			searchTasks:      make([]deferredSearchTask, 0, expectedPerWorker),
			newItemsMeta:     make([]*models.ContentMeta, 0, expectedPerWorker),
			newSearchRecords: make(map[string]*models.SearchRecord),
			newDependencies:  make(map[string]*models.Dependencies),
		}
	}
	var workerIdx atomic.Int32

	parsePool := async.NewWorkerPool(workerCtx.Ctx, numWorkers, func(file models.ScannedResource) error {
		idx := int(workerIdx.Add(1)-1) % numWorkers
		locals[idx].mu.Lock()
		defer locals[idx].mu.Unlock()
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

func (service *contentService) setupNavPages(nav navInfo, link string) (prev, next *models.NavPage) {
	if position, ok := nav.postPos[link]; ok {
		allItems := nav.allItems
		if position > 0 {
			prev = &models.NavPage{Title: allItems[position-1].Title, Link: allItems[position-1].Link}
		}
		if position < len(allItems)-1 {
			next = &models.NavPage{Title: allItems[position+1].Title, Link: allItems[position+1].Link}
		}
	}
	return
}

func (service *contentService) getPageLayout(renderTaskInstance renderTask) string {
	layoutVal := strings.ToLower(renderTaskInstance.file.Layout)
	if layoutVal == "" {
		if l, ok := renderTaskInstance.parseResult.Metadata["layout"].(string); ok {
			layoutVal = strings.ToLower(l)
		}
	}
	return layoutVal
}

func (service *contentService) runStreamingRenderPhase(ctx context.Context, numWorkers int, nav navInfo, tasks []renderTask, renderedMath map[string]string, renderedD2 map[string]models.SSRThemePair) {
	processed := atomic.Int32{}
	totalFiles := len(tasks)

	renderPool := async.NewWorkerPool(ctx, numWorkers, func(renderTaskInstance renderTask) error {
		return service.renderSingleTask(renderTaskInstance, renderedMath, renderedD2, nav, &processed, totalFiles)
	}).WithScheduler(service.ctx.Scheduler, scheduler.TaskMarkdown)

	renderPool.Start()
	for _, renderTaskInstance := range tasks {
		renderPool.Submit(renderTaskInstance)
	}
	if err := renderPool.Stop(); err != nil {
		service.logger.Error("Failed to stop render pool", "error", err)
	}
}

func (service *contentService) renderSingleTask(renderTaskInstance renderTask, renderedMath map[string]string, renderedD2 map[string]models.SSRThemePair, nav navInfo, processed *atomic.Int32, totalFiles int) error {
	item := renderTaskInstance.parseResult.Item
	htmlContent := renderTaskInstance.htmlContent
	relPrefix := fspkg.GetRelativePrefix(renderTaskInstance.htmlRelativePath)
	htmlContent = rewriteStaticAssetPaths(htmlContent, relPrefix)

	title, _ := renderTaskInstance.parseResult.Metadata["title"].(string)
	description, _ := renderTaskInstance.parseResult.Metadata["description"].(string)
	if title == "" {
		title = item.Title
	}
	if description == "" {
		description = item.Description
	}
	socialHash := generators.SocialCardHash(title, description, &service.cfg.SocialCards)

	_, _, cardImageURL := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, renderTaskInstance.htmlRelativePath, socialHash)
	prev, next := service.setupNavPages(nav, renderTaskInstance.file.Link)
	sectionIndexURL := navigation.ResolveSectionIndex(renderTaskInstance.htmlRelativePath)

	layoutVal := service.getPageLayout(renderTaskInstance)
	pageContext := models.ContextSection
	if layoutVal == "home" {
		pageContext = models.ContextHome
	}

	tabTitle := item.Title + " | " + service.cfg.Title
	if pageContext == models.ContextHome {
		tabTitle = service.cfg.Title
	}

	if err := service.renderer.RenderPage(renderTaskInstance.destinationPath, models.PageData{
		Title: item.Title, Description: item.Description, Content: template.HTML(htmlContent),
		Meta: renderTaskInstance.parseResult.Metadata, BaseURL: service.cfg.BaseURL, BuildVersion: service.cfg.BuildVersion,
		TabTitle: tabTitle, Permalink: renderTaskInstance.file.Link, Image: cardImageURL,
		SocialHash: socialHash,
		TOC:        renderTaskInstance.parseResult.TOC, Config: service.cfg, ReadingTime: item.ReadingTime,
		Taxonomies: nav.taxonomies, ItemTaxonomies: item.Taxonomies,
		PrevPage: prev, NextPage: next, RelativePrefix: relPrefix,
		HasImages: renderTaskInstance.parseResult.HasImages, Context: pageContext,
		ContentPrefix:   service.cfg.ContentPrefix,
		SectionIndexURL: sectionIndexURL,
		SiteData:        service.cfg.SiteData,
		JSONLD:          service.generateJSONLD(item, cardImageURL),
		Section:         item.Section,
		IsCleanBuild:    service.ctx.IsCleanBuild,
		SSRMath:         renderedMath,
		SSRD2:           renderedD2,
	}); err != nil {
		return err
	}

	if service.cfg.Features.UseRawMarkdown {
		service.handleRawMarkdown(renderTaskInstance.destinationPath, renderTaskInstance.source)
	}

	if service.reporter != nil {
		curr := int(processed.Add(1))
		service.reporter.UpdateProgress(ui.PhasePosts, curr, totalFiles, renderTaskInstance.relativePath)
	}
	return nil
}
