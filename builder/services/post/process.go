package post

import (
	"context"
	"html/template"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
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
func (s *postService) Process(opts ProcessOptions) (*PostResult, error) {
	fileChan := make(chan models.ScannedFile, len(opts.Files))
	for _, f := range opts.Files {
		fileChan <- f
	}
	close(fileChan)
	opts.FileChan = fileChan
	return s.ProcessStreaming(opts)
}

// ProcessStreaming processes posts using streaming parse and render phases.
func (s *postService) ProcessStreaming(opts ProcessOptions) (*PostResult, error) {
	ctx := opts.Ctx
	shouldForce := opts.ShouldForce
	forceSocialRebuild := opts.ForceSocialRebuild
	fileChan := opts.FileChan

	numWorkers := models.GetDefaultWorkerCount()

	cardPool := async.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) error {
		s.generateSocialCard(task)
		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskSocialCard)
	cardPool.Start()
	defer func() { buildCtx.IgnoreError(cardPool.Stop(), "stop card pool") }()

	// Search Indexing Pool (Background)
	searchPool := async.NewWorkerPool(ctx, numWorkers, func(task searchTask) error {
		wordFreqs, docLen, stemMap, posIndex, byteOffsets := tokenizeSearchData(task.record, task.plainText)

		// Update in-memory index result
		task.indexed.WordFreqs = wordFreqs
		task.indexed.DocLen = docLen
		task.indexed.StemMap = stemMap
		task.indexed.PositionalIndex = posIndex
		task.indexed.ByteOffsets = byteOffsets

		// Update BoltDB cache record if present
		if task.cached != nil {
			task.cached.BM25Data = wordFreqs
			task.cached.DocLen = docLen
			task.cached.StemMap = stemMap
			task.cached.PositionalIndex = posIndex
			task.cached.ByteOffsets = byteOffsets
		}
		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskMarkdown)
	searchPool.Start()
	defer func() { buildCtx.IgnoreError(searchPool.Stop(), "stop search pool") }()

	// Create a channel for navInfo to be sent once scanner finishes
	navReady := make(chan navInfo, navReadyBuffer)
	renderChan := make(chan renderTask, numWorkers*renderChanMultiplier)
	pc := &postProcessContext{
		tagMap:           make(map[string][]models.PostMetadata),
		newSearchRecords: make(map[string]*models.SearchRecord),
		newDeps:          make(map[string]*models.Dependencies),
		indexedPosts:     make([]models.IndexedPost, 0, indexedPostsCap),
	}

	// Internal channel to collect all files for navigation calculation
	collectedFilesChan := make(chan models.ScannedFile, collectedFilesBuffer)
	var allFiles []models.ScannedFile
	var collectWg sync.WaitGroup
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	collectWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "collect scan results",
		Fn: func() error {
			for f := range collectedFilesChan {
				allFiles = append(allFiles, f)
			}
			// Once all files collected, prepare navigation and signal render phase
			navReady <- s.prepareNavigationInfo(allFiles)
			return nil
		},
		Cleanup: collectWg.Done,
	})

	// Start render task collector goroutine to avoid blocking parse workers
	var renderTasks []renderTask
	var renderTasksMu sync.Mutex
	renderTasksDone := make(chan struct{})

	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "collect render tasks",
		Fn: func() error {
			for rt := range renderChan {
				renderTasksMu.Lock()
				renderTasks = append(renderTasks, rt)
				renderTasksMu.Unlock()
			}
			return nil
		},
		Cleanup: func() { close(renderTasksDone) },
	})

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

	wCtx := WorkerContext{
		Ctx: ctx, PC: pc, CardPool: cardPool, SearchPool: searchPool,
		RenderChan: renderChan, ShouldForce: shouldForce, ForceSocialRebuild: forceSocialRebuild,
	}

	// Stream from original channel to both parse workers and our navigation collector
	err := s.runStreamingParsePhase(numWorkers, fileChan, collectedFilesChan, wCtx)
	close(collectedFilesChan)
	collectWg.Wait() // Wait for scanner walk to finish and navReady to be filled

	// Once scanner is done and navReady is filled, signal the render phase
	close(renderChan) // No more render tasks will be produced
	<-renderTasksDone // Wait for all produced tasks to be collected

	// Now start the render pool with complete navInfo
	nav := <-navReady
	s.runStreamingRenderPhase(ctx, numWorkers, nav, renderTasks)

	renderWg.Wait() // Reserved for backward compatibility if needed, but runStreamingRenderPhase is now synchronous here

	if err != nil {
		return nil, err
	}

	// Wait for search indexing to complete before returning results
	if err := searchPool.Stop(); err != nil {
		s.logger.Error("Failed to stop search pool", "error", err)
	}

	s.finalizeBuild(pc)

	// Sort allPosts to ensure deterministic ordering across builds
	timeutil.SortPosts(pc.allPosts)
	timeutil.SortPosts(pc.pinnedPosts)
	// Sort tagMap values (slices of posts) for deterministic output
	for _, posts := range pc.tagMap {
		timeutil.SortPosts(posts)
	}

	return &PostResult{
		AllPosts: pc.allPosts, PinnedPosts: pc.pinnedPosts, TagMap: pc.tagMap,
		IndexedPosts: pc.indexedPosts, AnyPostChanged: pc.anyPostChanged.Load(), Has404: false,
	}, nil
}

func (s *postService) runStreamingParsePhase(numWorkers int, fileChan <-chan models.ScannedFile, collector chan<- models.ScannedFile, wCtx WorkerContext) error {
	s.logger.Info("Processing posts (streaming mode)")
	timer := timeutil.StartPhase("Process posts (stream)")
	defer timer.Stop()

	locals := make([]*workerLocalState, numWorkers)
	for i := range locals {
		locals[i] = &workerLocalState{
			newSearchRecs: make(map[string]*models.SearchRecord),
			newDeps:       make(map[string]*models.Dependencies),
		}
	}
	var workerIdx atomic.Int32

	parsePool := async.NewWorkerPool(wCtx.Ctx, numWorkers, func(f models.ScannedFile) error {
		id := int(workerIdx.Add(1)-1) % numWorkers
		s.parseWorkerTaskLocal(f, wCtx, locals[id])
		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskMarkdown)

	parsePool.Start()
	for f := range fileChan {
		if collector != nil {
			collector <- f
		}
		parsePool.Submit(f)
	}
	err := parsePool.Stop()

	s.mergeWorkerStates(locals, wCtx)

	return err
}

func (s *postService) runStreamingRenderPhase(ctx context.Context, numWorkers int, nav navInfo, tasks []renderTask) {
	processed := atomic.Int32{}
	totalFiles := len(tasks)

	renderPool := async.NewWorkerPool(ctx, numWorkers, func(rt renderTask) error {
		post := rt.parseRes.Post
		_, _, cardImageURL := navigation.CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, rt.htmlRelPath)
		var prev, next *models.NavPage
		if pos, ok := nav.postPos[rt.f.Link]; ok {
			vp := nav.allPosts
			if pos > 0 {
				prev = &models.NavPage{Title: vp[pos-1].Title, Link: vp[pos-1].Link}
			}
			if pos < len(vp)-1 {
				next = &models.NavPage{Title: vp[pos+1].Title, Link: vp[pos+1].Link}
			}
		}

		if err := s.renderer.RenderPage(rt.destPath, models.PageData{
			Title: post.Title, Description: post.Description, Content: template.HTML(rt.htmlContent),
			Meta: rt.parseRes.Metadata, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
			TabTitle: post.Title + " | " + s.cfg.Title, Permalink: rt.f.Link, Image: cardImageURL,
			TOC: rt.parseRes.TOC, Config: s.cfg, ReadingTime: post.ReadingTime,
			PrevPage: prev, NextPage: next, RelativePrefix: fspkg.GetRelativePrefix(rt.htmlRelPath),
			HasImages: rt.parseRes.HasImages,
			JSONLD:    models.GeneratePostJSONLD(post, s.cfg.Author, cardImageURL),
		}); err != nil {
			return err
		}

		if s.cfg.Features.RawMarkdown {
			mdDestPath := rt.destPath[:len(rt.destPath)-len(filepath.Ext(rt.destPath))] + ".md"
			if err := s.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
				s.logger.Warn("Failed to create directory for raw markdown", "dir", filepath.Dir(mdDestPath), "error", err)
			}
			if err := s.sink.WriteFile(mdDestPath, rt.source); err == nil {
				s.renderer.RegisterFile(mdDestPath)
			}
		}

		if s.reporter != nil {
			curr := int(processed.Add(1))
			s.reporter.UpdateProgress(ui.PhasePosts, curr, totalFiles, rt.relPath)
		}

		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskMarkdown)

	renderPool.Start()
	for _, rt := range tasks {
		renderPool.Submit(rt)
	}
	if err := renderPool.Stop(); err != nil {
		s.logger.Error("Failed to stop render pool", "error", err)
	}
}
