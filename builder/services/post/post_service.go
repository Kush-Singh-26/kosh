package post

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// postService implements PostService
type postService struct {
	ctx            *buildCtx.BuildContext
	cfg            *config.Config
	cache          Cache
	renderer       render.Service
	logger         *slog.Logger
	metrics        *metrics.BuildMetrics
	mdPool         *sync.Pool
	nativeRenderer *native.Renderer
	sourceFs       afero.Fs
	sink           fspkg.ArtifactSink
	reporter       ui.Reporter
	assetsReady    <-chan struct{}
	diagramAdapter *cache.DiagramCacheAdapter
	cacheWg        sync.WaitGroup
}

func NewService(deps Dependencies) Service {
	return &postService{
		ctx:            deps.Ctx,
		cfg:            deps.Cfg,
		cache:          deps.Cache,
		renderer:       deps.Renderer,
		logger:         deps.Logger,
		metrics:        deps.Metrics,
		mdPool:         deps.MdPool,
		nativeRenderer: deps.NativeRenderer,
		sourceFs:       deps.SourceFs,
		sink:           deps.Sink,
		reporter:       deps.Reporter,
		diagramAdapter: deps.DiagramAdapter,
	}
}

func (s *postService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

func (s *postService) SetAssetsGate(ch <-chan struct{}) { s.assetsReady = ch }
func (s *postService) ReconfigureWithReporter(r ui.Reporter, l *slog.Logger) {
	s.reporter = r
	s.logger = l
}
func (s *postService) WaitForCacheCommit() { s.cacheWg.Wait() }

type renderTask struct {
	parseRes    *ParsedMarkdownResult
	f           models.ScannedFile
	htmlContent string
	destPath    string
	relPath     string
	htmlRelPath string
	source      []byte
}

type searchTask struct {
	record    models.PostRecord
	plainText string
	indexed   *models.IndexedPost
	cached    *models.SearchRecord
}

// workerLocalState accumulates results within a single parse worker,
// eliminating contention on the shared postProcessContext.
type workerLocalState struct {
	allPosts      []models.PostMetadata
	pinnedPosts   []models.PostMetadata
	tagEntries    []tagEntry
	indexedPosts  []models.IndexedPost
	searchTasks   []deferredSearchTask
	newPostsMeta  []*models.PostMeta
	newSearchRecs map[string]*models.SearchRecord
	newDeps       map[string]*models.Dependencies
	anyChanged    bool
	errs          []error
}

type tagEntry struct {
	tag  string
	post models.PostMetadata
}

type deferredSearchTask struct {
	record    models.PostRecord
	plainText string
	localIdx  int // index into this worker's indexedPosts
	cached    *models.SearchRecord
}

// WorkerContext holds shared dependencies and configuration for streaming workers.
type WorkerContext struct {
	Ctx                context.Context
	PC                 *postProcessContext
	CardPool           *async.WorkerPool[socialCardTask]
	SearchPool         *async.WorkerPool[searchTask]
	RenderChan         chan<- renderTask
	ShouldForce        bool
	ForceSocialRebuild bool
}

func (s *postService) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, files []models.ScannedFile) (*PostResult, error) {
	fileChan := make(chan models.ScannedFile, len(files))
	for _, f := range files {
		fileChan <- f
	}
	close(fileChan)
	return s.ProcessStreaming(ctx, shouldForce, forceSocialRebuild, outputMissing, fileChan)
}

func (s *postService) ProcessStreaming(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*PostResult, error) {
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
	navReady := make(chan navInfo, 1)
	renderChan := make(chan renderTask, numWorkers*2)
	pc := &postProcessContext{
		tagMap:           make(map[string][]models.PostMetadata),
		newSearchRecords: make(map[string]*models.SearchRecord),
		newDeps:          make(map[string]*models.Dependencies),
		indexedPosts:     make([]models.IndexedPost, 0, 50),
	}

	// Internal channel to collect all files for navigation calculation
	collectedFilesChan := make(chan models.ScannedFile, 1024)
	var allFiles []models.ScannedFile
	var collectWg sync.WaitGroup
	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		for f := range collectedFilesChan {
			allFiles = append(allFiles, f)
		}
		// Once all files collected, prepare navigation and signal render phase
		navReady <- s.prepareNavigationInfo(allFiles)
	}()

	// Start render task collector goroutine to avoid blocking parse workers
	var renderTasks []renderTask
	var renderTasksMu sync.Mutex
	renderTasksDone := make(chan struct{})

	go func() {
		defer close(renderTasksDone)
		for rt := range renderChan {
			renderTasksMu.Lock()
			renderTasks = append(renderTasks, rt)
			renderTasksMu.Unlock()
		}
	}()

	// renderWg is kept for context-based cleanup if needed, though runStreamingRenderPhase is synchronous
	var renderWg sync.WaitGroup
	renderWg.Add(1)
	go func() {
		defer renderWg.Done()
		// This goroutine will wait for renderChan to close and tasks to be collected elsewhere
	}()

	wCtx := WorkerContext{
		Ctx: ctx, PC: pc, CardPool: cardPool, SearchPool: searchPool,
		RenderChan: renderChan, ShouldForce: shouldForce, ForceSocialRebuild: forceSocialRebuild,
	}

	// Stream from original channel to both parse workers and our navigation collector
	err := s.runStreamingParsePhase(numWorkers, fileChan, collectedFilesChan, wCtx)
	close(collectedFilesChan)
	collectWg.Wait() // Wait for scanner walk to finish and navReady to be filled

	// Once scanner is done and navReady is filled, signal the render phase
	close(renderChan)      // No more render tasks will be produced
	<-renderTasksDone      // Wait for all produced tasks to be collected

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

type postProcessContext struct {
	allPosts         []models.PostMetadata
	pinnedPosts      []models.PostMetadata
	tagMap           map[string][]models.PostMetadata
	anyPostChanged   atomic.Bool
	newPostsMeta     []*models.PostMeta
	newSearchRecords map[string]*models.SearchRecord
	newDeps          map[string]*models.Dependencies
	indexedPosts     []models.IndexedPost
	errs             []error
}

func (s *postService) parseWorkerTaskLocal(f models.ScannedFile, wCtx WorkerContext, local *workerLocalState) {
	path := f.Path
	relPath := f.RelPath

	htmlRelPath, _, destPath := navigation.ComputePathVars(s.cfg.OutputDir, relPath)

	// 1. Check Cache
	cachedMeta, useCache := s.checkCache(relPath, f, wCtx.ShouldForce)

	var parseRes *ParsedMarkdownResult
	var htmlContent string
	var finalSSRHashes []string

	if useCache {
		parseRes, htmlContent, useCache = s.loadFromCache(cachedMeta, htmlRelPath)
		if useCache {
			finalSSRHashes = cachedMeta.SSRInputHashes
			if s.metrics != nil {
				s.metrics.IncrementCacheHit()
			}
		}
	}

	// 2. Math processing from cache (only if we have cached HTML content)
	if useCache && htmlContent != "" && len(parseRes.MathExpressions) > 0 {
		var mathOk bool
		htmlContent, mathOk = s.processCachedMath(htmlContent, parseRes.MathExpressions)
		useCache = useCache && mathOk
	}

	// 3. Full Parse if needed
	var sourceBytes []byte
	var err error

	if !useCache || s.cfg.Features.RawMarkdown {
		if f.SourceLoader != nil {
			sourceBytes, err = f.SourceLoader()
			if err != nil {
				s.logger.Error("Failed to load source", "path", path, "error", err)
				local.errs = append(local.errs, err)
				return
			}
		}
	}

	if !useCache {
		if s.metrics != nil {
			s.metrics.IncrementCacheMiss()
		}

		readingTime := f.ReadingTime
		if cachedMeta != nil && cachedMeta.BodyHash == f.BodyHash && cachedMeta.ReadingTime > 0 {
			readingTime = cachedMeta.ReadingTime
		}

		parseRes, err = ParseMarkdown(
			ParseConfig{
				Source:               sourceBytes,
				Path:                 path,
				CleanHtmlRelPath:     htmlRelPath,
				HtmlRelPath:          htmlRelPath,
				KnownFrontmatterHash: f.FrontmatterHash,
				KnownReadingTime:     readingTime,
				BodyOffset:           f.BodyOffset,
				PreParsedMeta:        f.PreParsedMeta,
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
			s.logger.Error("Failed to parse markdown", "path", path, "error", err)
			local.errs = append(local.errs, err)
			return
		}
		local.anyChanged = true

		if parseRes.Post.Title == "" {
			parseRes.Post.Title = f.Title
		}

		htmlContent = s.renderMath(wCtx.Ctx, path, parseRes)
		finalSSRHashes = parseRes.SSRHashes
	}

	post := parseRes.Post
	if post.Draft && !s.cfg.IncludeDrafts {
		return
	}

	// 4. Social Card
	s.queueSocialCard(relPath, parseRes, htmlRelPath, wCtx.ForceSocialRebuild, wCtx.CardPool)

	// 5. Aggregate and stream
	s.aggregateLocal(f, parseRes, post, htmlContent, destPath, relPath, htmlRelPath, finalSSRHashes, useCache, wCtx, local, sourceBytes)
	if s.metrics != nil {
		s.metrics.IncrementPostsProcessed()
	}
}

func (s *postService) aggregateLocal(f models.ScannedFile, res *ParsedMarkdownResult, post models.PostMetadata, htmlContent, destPath, relPath, htmlRelPath string, ssrHashes []string, useCache bool, wCtx WorkerContext, local *workerLocalState, sourceBytes []byte) {
	searchRecord := res.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)

	localIdx := len(local.indexedPosts)
	local.indexedPosts = append(local.indexedPosts, models.IndexedPost{
		Record: searchRecord, WordFreqs: res.WordFreqs, DocLen: res.DocLen,
		StemMap: res.StemMap, PositionalIndex: res.PositionalIndex, ByteOffsets: res.ByteOffsets,
	})

	if !useCache && s.cache != nil {
		newSearch := &models.SearchRecord{
			Title: post.Title, NormalizedTitle: res.SearchRecord.NormalizedTitle,
			Content: res.SearchRecord.Content, NormalizedTags: res.SearchRecord.NormalizedTags,
		}
		postID := cache.GeneratePostID("", relPath)
		local.newSearchRecs[postID] = newSearch

		if s.cfg.Features.Generators.Search {
			local.searchTasks = append(local.searchTasks, deferredSearchTask{
				record: searchRecord, plainText: res.PlainText, localIdx: localIdx, cached: newSearch,
			})
		}
	}

	local.allPosts = append(local.allPosts, post)
	if post.Pinned {
		local.pinnedPosts = append(local.pinnedPosts, post)
	}
	for _, tag := range post.Tags {
		local.tagEntries = append(local.tagEntries, tagEntry{
			tag: strings.ToLower(strings.TrimSpace(tag)), post: post,
		})
	}

	// Stream to renderer // No locks needed for channel write
	wCtx.RenderChan <- renderTask{
		parseRes:    res,
		f:           f,
		htmlContent: htmlContent,
		destPath:    destPath,
		relPath:     relPath,
		htmlRelPath: htmlRelPath,
		source:      sourceBytes,
	}

	if !useCache && s.cache != nil {
		postID := cache.GeneratePostID("", relPath)
		newMeta := &models.PostMeta{
			PostID: postID, Path: relPath, ModTime: f.Info.ModTime().UnixNano(),
			ContentHash: res.FrontmatterHash, BodyHash: f.BodyHash, Title: post.Title, Date: post.DateObj,
			WordCount: int(f.Info.Size()), // Use WordCount as size for quick comparison
			Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
			Meta: res.Metadata, TOC: res.TOC, SSRInputHashes: ssrHashes,
			CardHash: res.FrontmatterHash, HasImages: res.HasImages, MathExpressions: res.MathExpressions,
		}
		if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
			s.logger.Warn("Failed to store HTML for post", "path", relPath, "error", err)
		}
		local.newPostsMeta = append(local.newPostsMeta, newMeta)
		local.newDeps[postID] = &models.Dependencies{Tags: post.Tags}
	}
}

func (s *postService) mergeWorkerStates(locals []*workerLocalState, wCtx WorkerContext) {
	pc := wCtx.PC
	for _, local := range locals {
		if local.anyChanged {
			pc.anyPostChanged.Store(true)
		}
		baseIdx := len(pc.indexedPosts)
		pc.allPosts = append(pc.allPosts, local.allPosts...)
		pc.pinnedPosts = append(pc.pinnedPosts, local.pinnedPosts...)
		pc.indexedPosts = append(pc.indexedPosts, local.indexedPosts...)
		pc.newPostsMeta = append(pc.newPostsMeta, local.newPostsMeta...)
		for k, v := range local.newSearchRecs {
			pc.newSearchRecords[k] = v
		}
		for k, v := range local.newDeps {
			pc.newDeps[k] = v
		}
		for _, te := range local.tagEntries {
			pc.tagMap[te.tag] = append(pc.tagMap[te.tag], te.post)
		}
		pc.errs = append(pc.errs, local.errs...)

		for _, st := range local.searchTasks {
			globalIdx := baseIdx + st.localIdx
			wCtx.SearchPool.Submit(searchTask{
				record:    st.record,
				plainText: st.plainText,
				indexed:   &pc.indexedPosts[globalIdx],
				cached:    st.cached,
			})
		}
	}
}

func (s *postService) queueSocialCard(relPath string, res *ParsedMarkdownResult, htmlRelPath string, force bool, pool *async.WorkerPool[socialCardTask]) {
	cardRelPath, cardDestPath, _ := navigation.CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, htmlRelPath)

	cacheDir := s.cfg.CacheDir
	if !filepath.IsAbs(cacheDir) {
		abs, err := filepath.Abs(cacheDir)
		if err == nil {
			cacheDir = abs
		}
	}
	cachedCardPath := filepath.Join(cacheDir, "social-cards", res.FrontmatterHash+".webp")

	if _, err := os.Stat(cachedCardPath); err == nil && !force && res.FrontmatterHash != "" {
		s.copyCachedSocialCard(res.FrontmatterHash, cardDestPath)
	} else {
		pool.Submit(socialCardTask{
			path: relPath, relPath: cardRelPath,
			cardDestPath: cardDestPath, metadata: res.Metadata, frontmatterHash: res.FrontmatterHash,
		})
	}
}

func (s *postService) copyCachedSocialCard(cardHash, cardDestPath string) {
	if cardHash == "" {
		s.sink.Register(cardDestPath)
		return
	}
	cachedCardPath := filepath.Join(s.cfg.CacheDir, "social-cards", cardHash+".webp")
	cachedFile, err := os.Open(cachedCardPath)
	if err != nil {
		s.logger.Warn("Failed to open cached social card", "path", cachedCardPath, "error", err)
		s.sink.Register(cardDestPath)
		return
	}
	defer func() {
		if err := cachedFile.Close(); err != nil {
			s.logger.Warn("Failed to close cached social card", "path", cachedCardPath, "error", err)
		}
	}()

	if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
		s.logger.Warn("Failed to create social card directory", "path", filepath.Dir(cardDestPath), "error", err)
	}

	err = s.sink.WriteStream(cardDestPath, func(w io.Writer) error {
		_, err := io.Copy(w, cachedFile)
		return err
	})
	if err != nil {
		s.logger.Warn("Failed to copy cached social card", "path", cardDestPath, "error", err)
		return
	}
	s.renderer.RegisterFile(cardDestPath)
}

func (s *postService) finalizeBuild(pc *postProcessContext) {
	if len(pc.newPostsMeta) > 0 && s.cache != nil {
		s.cacheWg.Add(1)
		async.FireAndForgetWithCleanup(context.Background(), s.logger, "cache commit",
			func() error {
				return s.cache.BatchCommit(pc.newPostsMeta, pc.newSearchRecords, pc.newDeps)
			},
			func() {
				s.cacheWg.Done()
			})
	}
}
