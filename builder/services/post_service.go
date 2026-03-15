package services

import (
	"context"
	"html/template"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

type PostServiceDependencies struct {
	Cfg            *config.Config
	Cache          CacheService
	Renderer       RenderService
	Logger         *slog.Logger
	Metrics        *metrics.BuildMetrics
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
	SourceFs       afero.Fs
	Sink           utils.ArtifactSink
	DiagramAdapter *cache.DiagramCacheAdapter
}

type postServiceImpl struct {
	cfg              *config.Config
	cache            CacheService
	renderer         RenderService
	logger           *slog.Logger
	metrics          *metrics.BuildMetrics
	mdPool           *sync.Pool
	nativeRenderer   *native.Renderer
	sourceFs         afero.Fs
	sink             utils.ArtifactSink
	assetsReady      <-chan struct{}
	diagramAdapter   *cache.DiagramCacheAdapter
	metadataReadyCh  chan struct{}  // Channel-based notification for explicit orchestration
	cardHashes       sync.Map
	logoExists       sync.Map
	cacheWg          sync.WaitGroup
	mu               sync.Mutex
}

func NewPostService(deps PostServiceDependencies) PostService {
	return &postServiceImpl{
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

func (s *postServiceImpl) ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
	// Create fresh channel for this build
	s.metadataReadyCh = make(chan struct{})
}

func (s *postServiceImpl) SetAssetsGate(ch <-chan struct{})    { s.assetsReady = ch }
func (s *postServiceImpl) MetadataReadyChan() <-chan struct{}  { return s.metadataReadyCh }
func (s *postServiceImpl) WaitForCacheCommit()                 { s.cacheWg.Wait() }

type renderTask struct {
	parseRes    *ParsedMarkdownResult
	f           models.ScannedFile
	htmlContent string
	destPath    string
	version     string
	relPath     string
	htmlRelPath string
}

func (s *postServiceImpl) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*PostResult, error) {
	numWorkers := utils.GetDefaultWorkerCount()

	cardPool := utils.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) {
		s.generateSocialCard(task)
	}).WithScheduler(utils.GlobalScheduler, utils.TaskSocialCard)
	cardPool.Start()
	defer cardPool.Stop()

	pc := s.runParsePhase(ctx, numWorkers, shouldForce, forceSocialRebuild, fileChan, cardPool)
	if len(pc.errs) > 0 {
		return nil, pc.errs[0]
	}

	utils.SortPosts(pc.allPosts)
	
	// Signal metadata ready via channel (explicit orchestration)
	close(s.metadataReadyCh)

	s.runRenderPhase(ctx, numWorkers, pc)

	s.finalizeBuild(pc)

	return &PostResult{
		AllPosts: pc.allPosts, PinnedPosts: pc.pinnedPosts, TagMap: pc.tagMap,
		IndexedPosts: pc.indexedPosts, AnyPostChanged: pc.anyPostChanged.Load(), Has404: false,
	}, nil
}

type postProcessContext struct {
	allPosts         []models.PostMetadata
	pinnedPosts      []models.PostMetadata
	tagMap           map[string][]models.PostMetadata
	anyPostChanged   atomic.Bool
	readyToRender    []renderTask
	newPostsMeta     []*cache.PostMeta
	newSearchRecords map[string]*cache.SearchRecord
	newDeps          map[string]*cache.Dependencies
	indexedPosts     []models.IndexedPost
	errs             []error
	mu               sync.Mutex
}

func (s *postServiceImpl) runParsePhase(ctx context.Context, numWorkers int, shouldForce, forceSocialRebuild bool, fileChan <-chan models.ScannedFile, cardPool *utils.WorkerPool[socialCardTask]) *postProcessContext {
	pc := &postProcessContext{
		tagMap:           make(map[string][]models.PostMetadata),
		newSearchRecords: make(map[string]*cache.SearchRecord),
		newDeps:          make(map[string]*cache.Dependencies),
	}

	s.logger.Info("Processing posts (streaming mode)")
	timer := utils.StartPhase("Process posts (stream)")
	defer timer.Stop()

	parsePool := utils.NewWorkerPool(ctx, numWorkers, func(f models.ScannedFile) {
		s.parseWorkerTask(ctx, f, shouldForce, forceSocialRebuild, pc, cardPool)
	}).WithScheduler(utils.GlobalScheduler, utils.TaskMarkdown)

	parsePool.Start()
	for f := range fileChan {
		parsePool.Submit(f)
	}
	parsePool.Stop()

	return pc
}

func (s *postServiceImpl) parseWorkerTask(ctx context.Context, f models.ScannedFile, shouldForce, forceSocialRebuild bool, pc *postProcessContext, cardPool *utils.WorkerPool[socialCardTask]) {
	path, version := f.Path, f.Version
	relPath, _ := utils.SafeRel(s.cfg.ContentDir, path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

	versionLower := strings.ToLower(version)
	cleanHtmlRelPath := htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, versionLower+"/")
	}

	destPath := filepath.Join(s.cfg.OutputDir, htmlRelPath)
	if version != "" {
		destPath = filepath.Join(s.cfg.OutputDir, version, cleanHtmlRelPath)
	}

	// 1. Check Cache
	cachedMeta, useCache := s.checkCache(relPath, f, shouldForce)

	var parseRes *ParsedMarkdownResult
	var htmlContent string
	var finalSSRHashes []string

	if useCache {
		parseRes, htmlContent, useCache = s.loadFromCache(cachedMeta, htmlRelPath)
		if useCache {
			finalSSRHashes = cachedMeta.SSRInputHashes
			s.metrics.IncrementCacheHit()
		}
	}

	// 2. Math processing from cache
	if useCache && s.diagramAdapter != nil && len(parseRes.MathExpressions) > 0 {
		htmlContent = s.processCachedMath(htmlContent, parseRes.MathExpressions)
	}

	// 3. Full Parse if needed
	if !useCache {
		var err error
		s.metrics.IncrementCacheMiss()
		parseRes, err = ParseMarkdown(ctx, f.Source, path, version, cleanHtmlRelPath, htmlRelPath, s.mdPool, s.cfg, s.nativeRenderer, s.diagramAdapter, &s.mu, f.FrontmatterHash, f.ReadingTime, f.BodyOffset, f.PreParsedMeta)
		if err != nil {
			s.logger.Error("Failed to parse markdown", "path", path, "error", err)
			pc.mu.Lock()
			pc.errs = append(pc.errs, err)
			pc.mu.Unlock()
			return
		}
		pc.anyPostChanged.Store(true)

		htmlContent = s.renderMath(ctx, path, parseRes)
		finalSSRHashes = parseRes.SSRHashes
	}

	post := parseRes.Post
	if post.Draft && !s.cfg.IncludeDrafts {
		return
	}

	// 4. Social Card
	s.queueSocialCard(relPath, parseRes, htmlRelPath, forceSocialRebuild, cardPool)

	// 5. Aggregate Results
	s.aggregateParseResult(pc, f, parseRes, post, htmlContent, destPath, version, relPath, htmlRelPath, finalSSRHashes, useCache)
	s.metrics.IncrementPostsProcessed()
}

func (s *postServiceImpl) checkCache(relPath string, f models.ScannedFile, shouldForce bool) (*cache.PostMeta, bool) {
	if s.cache == nil || shouldForce {
		return nil, false
	}
	cachedMeta, err := s.cache.GetPostByPath(relPath)
	if err != nil || cachedMeta == nil {
		return nil, false
	}
	fastBail := cachedMeta.BodyHash == f.BodyHash && cachedMeta.ContentHash == f.FrontmatterHash
	return cachedMeta, fastBail
}

func (s *postServiceImpl) loadFromCache(cachedMeta *cache.PostMeta, htmlRelPath string) (*ParsedMarkdownResult, string, bool) {
	cachedHTML, err := s.cache.GetHTMLContent(cachedMeta)
	if err != nil || cachedHTML == nil {
		return nil, "", false
	}
	cachedSearch, err := s.cache.GetSearchRecord(cachedMeta.PostID)
	if err != nil || cachedSearch == nil {
		return nil, "", false
	}

	res := &ParsedMarkdownResult{
		MetaData: cachedMeta.Meta, TOC: cachedMeta.TOC,
		FrontmatterHash: cachedMeta.ContentHash, SSRHashes: cachedMeta.SSRInputHashes,
		HasImages: cachedMeta.HasImages, MathExpressions: cachedMeta.MathExpressions,
		SearchRecord: models.PostRecord{
			ID:    xxh3.HashString(cachedMeta.Link),
			Title: cachedSearch.Title, NormalizedTitle: cachedSearch.NormalizedTitle,
			Link: htmlRelPath, Content: cachedSearch.Content,
			NormalizedTags: cachedSearch.NormalizedTags, Version: cachedMeta.Version,
		},
		WordFreqs: cachedSearch.BM25Data, DocLen: cachedSearch.DocLen,
		StemMap: cachedSearch.StemMap, PositionalIndex: cachedSearch.PositionalIndex,
		ByteOffsets: cachedSearch.ByteOffsets,
		Post: models.PostMetadata{
			Title: cachedMeta.Title, Link: cachedMeta.Link, Description: cachedMeta.Description,
			Tags: cachedMeta.Tags, Pinned: cachedMeta.Pinned, Weight: cachedMeta.Weight,
			ReadingTime: cachedMeta.ReadingTime, DateObj: cachedMeta.Date,
			Draft: cachedMeta.Draft, Version: cachedMeta.Version,
		},
	}
	return res, string(cachedHTML), true
}

func (s *postServiceImpl) processCachedMath(html string, exprs []models.MathExpression) string {
	renderedMath := make(map[string]string)
	for _, expr := range exprs {
		if v, ok := s.diagramAdapter.GetLocal(expr.Hash); ok {
			renderedMath[expr.Hash] = v
		}
	}
	if len(renderedMath) > 0 {
		return mdParser.ReplaceMathExpressions(html, exprs, renderedMath)
	}
	return html
}

func (s *postServiceImpl) renderMath(ctx context.Context, path string, res *ParsedMarkdownResult) string {
	if len(res.MathExpressions) == 0 {
		return res.HTMLContent
	}

	cachedSubset := make(map[string]string)
	if s.diagramAdapter != nil {
		for _, e := range res.MathExpressions {
			if v, ok := s.diagramAdapter.GetLocal(e.Hash); ok {
				cachedSubset[e.Hash] = v
			}
		}
	}

	rendered, err := s.nativeRenderer.RenderAllMath(ctx, res.MathExpressions, cachedSubset)
	if err != nil {
		s.logger.Warn("Math render failed for post", "path", path, "error", err)
	}

	if s.diagramAdapter != nil && len(rendered) > 0 {
		newMath := make(map[string]string)
		for h, v := range rendered {
			if _, ok := cachedSubset[h]; !ok {
				newMath[h] = v
			}
		}
		if len(newMath) > 0 {
			s.diagramAdapter.Merge(newMath)
		}
	}

	return mdParser.ReplaceMathExpressions(res.HTMLContent, res.MathExpressions, rendered)
}

func (s *postServiceImpl) queueSocialCard(relPath string, res *ParsedMarkdownResult, htmlRelPath string, force bool, pool *utils.WorkerPool[socialCardTask]) {
	cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp"))
	cardHash, _ := s.cardHashes.Load(relPath)
	if force || cardHash != res.FrontmatterHash {
		pool.Submit(socialCardTask{
			path: relPath, relPath: strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
			cardDestPath: cardDestPath, metaData: res.MetaData, frontmatterHash: res.FrontmatterHash,
		})
	} else {
		s.sink.Register(cardDestPath)
	}
}

func (s *postServiceImpl) aggregateParseResult(pc *postProcessContext, f models.ScannedFile, res *ParsedMarkdownResult, post models.PostMetadata, htmlContent, destPath, version, relPath, htmlRelPath string, ssrHashes []string, useCache bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	searchRecord := res.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)
	pc.indexedPosts = append(pc.indexedPosts, models.IndexedPost{
		Record: searchRecord, WordFreqs: res.WordFreqs, DocLen: res.DocLen,
		StemMap: res.StemMap, PositionalIndex: res.PositionalIndex, ByteOffsets: res.ByteOffsets,
	})

	pc.allPosts = append(pc.allPosts, post)
	if post.Pinned {
		pc.pinnedPosts = append(pc.pinnedPosts, post)
	}
	for _, tag := range post.Tags {
		k := strings.ToLower(strings.TrimSpace(tag))
		pc.tagMap[k] = append(pc.tagMap[k], post)
	}

	pc.readyToRender = append(pc.readyToRender, renderTask{
		parseRes: res, f: f, htmlContent: htmlContent,
		destPath: destPath, version: version, relPath: relPath, htmlRelPath: htmlRelPath,
	})

	if !useCache && s.cache != nil {
		postID := cache.GeneratePostID("", relPath)
		newMeta := &cache.PostMeta{
			PostID: postID, Path: relPath, ModTime: f.Info.ModTime().Unix(),
			ContentHash: res.FrontmatterHash, BodyHash: f.BodyHash, Title: post.Title, Date: post.DateObj,
			Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
			Meta: res.MetaData, TOC: res.TOC, Version: version, SSRInputHashes: ssrHashes,
			CardHash: res.FrontmatterHash, HasImages: res.HasImages, MathExpressions: res.MathExpressions,
		}
		_ = s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent))
		newSearch := &cache.SearchRecord{
			Title: post.Title, NormalizedTitle: res.SearchRecord.NormalizedTitle,
			BM25Data: res.WordFreqs, DocLen: res.DocLen, Content: res.SearchRecord.Content,
			NormalizedTags: res.SearchRecord.NormalizedTags, StemMap: res.StemMap,
			PositionalIndex: res.PositionalIndex, ByteOffsets: res.ByteOffsets,
		}
		pc.newPostsMeta = append(pc.newPostsMeta, newMeta)
		pc.newSearchRecords[postID] = newSearch
		pc.newDeps[postID] = &cache.Dependencies{Tags: post.Tags}
	}
}

func (s *postServiceImpl) runRenderPhase(ctx context.Context, numWorkers int, pc *postProcessContext) {
	// Prepare versioned navigation
	postsByVersion := make(map[string][]models.PostMetadata)
	postPosByVersion := make(map[string]map[string]int)
	for _, p := range pc.allPosts {
		postsByVersion[p.Version] = append(postsByVersion[p.Version], p)
	}
	for ver, posts := range postsByVersion {
		utils.SortPosts(posts)
		postPosByVersion[ver] = make(map[string]int)
		for i, p := range posts {
			postPosByVersion[ver][p.Link] = i
		}
	}

	renderPool := utils.NewWorkerPool(ctx, numWorkers, func(rt renderTask) {
		post := rt.parseRes.Post
		imagePath := s.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(rt.htmlRelPath, ".html") + ".webp"
		var prev, next *models.NavPage
		if pos, ok := postPosByVersion[rt.version][post.Link]; ok {
			vp := postsByVersion[rt.version]
			if pos > 0 {
				prev = &models.NavPage{Title: vp[pos-1].Title, Link: vp[pos-1].Link}
			}
			if pos < len(vp)-1 {
				next = &models.NavPage{Title: vp[pos+1].Title, Link: vp[pos+1].Link}
			}
		}

		s.renderer.RenderPage(rt.destPath, models.PageData{
			Title: post.Title, Description: post.Description, Content: template.HTML(rt.htmlContent),
			Meta: rt.parseRes.MetaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
			TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
			TOC: rt.parseRes.TOC, Config: s.cfg, CurrentVersion: rt.version, ReadingTime: post.ReadingTime,
			PrevPage: prev, NextPage: next, RelativePrefix: utils.GetRelativePrefix(rt.htmlRelPath),
			HasImages: rt.parseRes.HasImages,
		})
	}).WithScheduler(utils.GlobalScheduler, utils.TaskMarkdown)

	renderPool.Start()
	for _, rt := range pc.readyToRender {
		renderPool.Submit(rt)
	}
	renderPool.Stop()
}

// finalizeBuild commits cache changes asynchronously.
// Cache commits are fire-and-forget: errors are logged but don't fail the build,
// as the cache will rebuild on next run. This avoids blocking the build pipeline.
func (s *postServiceImpl) finalizeBuild(pc *postProcessContext) {
	if len(pc.newPostsMeta) > 0 && s.cache != nil {
		s.cacheWg.Add(1)
		go func() {
			defer s.cacheWg.Done()
			if err := s.cache.BatchCommit(pc.newPostsMeta, pc.newSearchRecords, pc.newDeps); err != nil {
				s.logger.Error("Cache batch commit failed", "error", err)
			}
		}()
	}
}
