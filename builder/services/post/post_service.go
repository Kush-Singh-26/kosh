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
		diagramAdapter: deps.DiagramAdapter,
	}
}

func (s *postService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

func (s *postService) SetAssetsGate(ch <-chan struct{}) { s.assetsReady = ch }
func (s *postService) WaitForCacheCommit()              { s.cacheWg.Wait() }

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

func (s *postService) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, files []models.ScannedFile) (*PostResult, error) {
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

	// Pre-calculate navigation info using fast-scanned file metadata
	navInfo := s.prepareNavigationInfo(files)

	renderChan := make(chan renderTask, numWorkers*2)
	pc := &postProcessContext{
		tagMap:           make(map[string][]models.PostMetadata),
		newSearchRecords: make(map[string]*models.SearchRecord),
		newDeps:          make(map[string]*models.Dependencies),
		indexedPosts:     make([]models.IndexedPost, 0, len(files)),
	}

	// Start render phase concurrently with parse phase (pipelining)
	var renderWg sync.WaitGroup
	renderWg.Add(1)
	go func() {
		defer renderWg.Done()
		s.runStreamingRenderPhase(ctx, numWorkers, navInfo, renderChan)
	}()

	err := s.runStreamingParsePhase(ctx, numWorkers, shouldForce, forceSocialRebuild, files, cardPool, searchPool, pc, renderChan)
	close(renderChan)
	renderWg.Wait()

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

func (s *postService) runStreamingParsePhase(ctx context.Context, numWorkers int, shouldForce, forceSocialRebuild bool, files []models.ScannedFile, cardPool *async.WorkerPool[socialCardTask], searchPool *async.WorkerPool[searchTask], pc *postProcessContext, renderChan chan<- renderTask) error {
	s.logger.Info("Processing posts (pipelined mode)")
	timer := timeutil.StartPhase("Process posts (stream)")
	defer timer.Stop()

	parsePool := async.NewWorkerPool(ctx, numWorkers, func(f models.ScannedFile) error {
		s.parseWorkerTaskStreaming(ctx, f, shouldForce, forceSocialRebuild, pc, cardPool, searchPool, renderChan)
		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskMarkdown)

	parsePool.Start()
	for _, f := range files {
		parsePool.Submit(f)
	}
	return parsePool.Stop()
}

func (s *postService) runStreamingRenderPhase(ctx context.Context, numWorkers int, nav navInfo, renderChan <-chan renderTask) {
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

		return nil
	}).WithScheduler(s.ctx.Scheduler, scheduler.TaskMarkdown)

	renderPool.Start()
	for rt := range renderChan {
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
	mu               sync.Mutex
}

func (s *postService) parseWorkerTaskStreaming(ctx context.Context, f models.ScannedFile, shouldForce, forceSocialRebuild bool, pc *postProcessContext, cardPool *async.WorkerPool[socialCardTask], searchPool *async.WorkerPool[searchTask], renderChan chan<- renderTask) {
	path := f.Path
	relPath := f.RelPath

	htmlRelPath, _, destPath := navigation.ComputePathVars(s.cfg.OutputDir, relPath)

	// 1. Check Cache
	cachedMeta, useCache := s.checkCache(relPath, f, shouldForce)

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
	if !useCache {
		var err error
		if s.metrics != nil {
			s.metrics.IncrementCacheMiss()
		}

		readingTime := f.ReadingTime
		if cachedMeta != nil && cachedMeta.BodyHash == f.BodyHash && cachedMeta.ReadingTime > 0 {
			readingTime = cachedMeta.ReadingTime
		}

		parseRes, err = ParseMarkdown(
			ParseConfig{
				Source:               f.Source,
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
			pc.mu.Lock()
			pc.errs = append(pc.errs, err)
			pc.mu.Unlock()
			return
		}
		pc.anyPostChanged.Store(true)

		if parseRes.Post.Title == "" {
			parseRes.Post.Title = f.Title
		}

		htmlContent = s.renderMath(ctx, path, parseRes)
		finalSSRHashes = parseRes.SSRHashes
	}

	post := parseRes.Post
	if post.Draft && !s.cfg.IncludeDrafts {
		return
	}

	// 4. Social Card
	s.queueSocialCard(relPath, parseRes, htmlRelPath, forceSocialRebuild, cardPool)

	// 5. Aggregate and stream
	s.aggregateAndStream(pc, f, parseRes, post, htmlContent, destPath, relPath, htmlRelPath, finalSSRHashes, useCache, renderChan, searchPool)
	if s.metrics != nil {
		s.metrics.IncrementPostsProcessed()
	}
}

func (s *postService) aggregateAndStream(pc *postProcessContext, f models.ScannedFile, res *ParsedMarkdownResult, post models.PostMetadata, htmlContent, destPath, relPath, htmlRelPath string, ssrHashes []string, useCache bool, renderChan chan<- renderTask, searchPool *async.WorkerPool[searchTask]) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	searchRecord := res.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)

	// Add to indexed posts
	idx := len(pc.indexedPosts)
	pc.indexedPosts = append(pc.indexedPosts, models.IndexedPost{
		Record: searchRecord, WordFreqs: res.WordFreqs, DocLen: res.DocLen,
		StemMap: res.StemMap, PositionalIndex: res.PositionalIndex, ByteOffsets: res.ByteOffsets,
	})

	var newSearch *models.SearchRecord
	if !useCache && s.cache != nil {
		newSearch = &models.SearchRecord{
			Title: post.Title, NormalizedTitle: res.SearchRecord.NormalizedTitle,
			Content: res.SearchRecord.Content, NormalizedTags: res.SearchRecord.NormalizedTags,
		}
		postID := cache.GeneratePostID("", relPath)
		pc.newSearchRecords[postID] = newSearch
	}

	// If search is enabled and NOT from cache, offload analysis
	if s.cfg.Features.Generators.Search && !useCache {
		searchPool.Submit(searchTask{
			record:    searchRecord,
			plainText: res.PlainText,
			indexed:   &pc.indexedPosts[idx],
			cached:    newSearch,
		})
	}

	pc.allPosts = append(pc.allPosts, post)
	if post.Pinned {
		pc.pinnedPosts = append(pc.pinnedPosts, post)
	}
	for _, tag := range post.Tags {
		k := strings.ToLower(strings.TrimSpace(tag))
		pc.tagMap[k] = append(pc.tagMap[k], post)
	}

	// Stream to renderer
	renderChan <- renderTask{
		parseRes:    res,
		f:           f,
		htmlContent: htmlContent,
		destPath:    destPath,
		relPath:     relPath,
		htmlRelPath: htmlRelPath,
		source:      f.Source,
	}

	if !useCache && s.cache != nil {
		postID := cache.GeneratePostID("", relPath)
		newMeta := &models.PostMeta{
			PostID: postID, Path: relPath, ModTime: f.Info.ModTime().Unix(),
			ContentHash: res.FrontmatterHash, BodyHash: f.BodyHash, Title: post.Title, Date: post.DateObj,
			Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
			Meta: res.Metadata, TOC: res.TOC, SSRInputHashes: ssrHashes,
			CardHash: res.FrontmatterHash, HasImages: res.HasImages, MathExpressions: res.MathExpressions,
		}
		if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
			s.logger.Warn("Failed to store HTML for post", "path", relPath, "error", err)
		}
		pc.newPostsMeta = append(pc.newPostsMeta, newMeta)
		pc.newDeps[postID] = &models.Dependencies{Tags: post.Tags}
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
	defer cachedFile.Close()

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
