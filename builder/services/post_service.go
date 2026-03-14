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
	metadataCallback MetadataReadyFunc
	cardHashes       sync.Map
	logoExists       sync.Map
	cacheWg          sync.WaitGroup
	mu               sync.Mutex
}

func NewPostService(cfg *config.Config, cache CacheService, rnd RenderService, logger *slog.Logger, m *metrics.BuildMetrics, mdPool *sync.Pool, nativeRnd *native.Renderer, sourceFs afero.Fs, sink utils.ArtifactSink, diagramAdapter *cache.DiagramCacheAdapter) PostService {
	return &postServiceImpl{
		cfg:            cfg,
		cache:          cache,
		renderer:       rnd,
		logger:         logger,
		metrics:        m,
		mdPool:         mdPool,
		nativeRenderer: nativeRnd,
		sourceFs:       sourceFs,
		sink:           sink,
		diagramAdapter: diagramAdapter,
	}
}

func (s *postServiceImpl) SetSink(sink utils.ArtifactSink)          { s.sink = sink }
func (s *postServiceImpl) SetSourceFs(fs afero.Fs)                  { s.sourceFs = fs }
func (s *postServiceImpl) SetAssetsGate(ch <-chan struct{})         { s.assetsReady = ch }
func (s *postServiceImpl) SetMetadataCallback(fn MetadataReadyFunc) { s.metadataCallback = fn }
func (s *postServiceImpl) WaitForCacheCommit()                      { s.cacheWg.Wait() }

func (s *postServiceImpl) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile, has404 bool) (*PostResult, error) {
	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		tagMap         = make(map[string][]models.PostMetadata)
		anyPostChanged atomic.Bool
	)

	numWorkers := utils.GetDefaultWorkerCount()

	cardPool := utils.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) {
		s.generateSocialCard(task)
	}).WithScheduler(utils.GlobalScheduler, utils.TaskSocialCard)
	cardPool.Start()

	var (
		batchMu          sync.Mutex
		newPostsMeta     []*cache.PostMeta
		newSearchRecords = make(map[string]*cache.SearchRecord)
		newDeps          = make(map[string]*cache.Dependencies)
		indexedPosts     []models.IndexedPost
		indexedMu        sync.Mutex
	)

	type renderTask struct {
		parseRes    *ParsedMarkdownResult
		f           models.ScannedFile
		htmlContent string
		destPath    string
		version     string
		relPath     string
		htmlRelPath string
	}
	var readyToRender []renderTask
	var renderMu sync.Mutex

	s.logger.Info("Processing posts (streaming mode)")
	processTimer := utils.StartPhase("Process posts (stream)")
	defer processTimer.Stop()

	var (
		phase1Errs []error
		phase1Mu   sync.Mutex
	)

	// Phase 1: Parsing and Metadata Extraction (Parallel with Scanner)
	parsePool := utils.NewWorkerPool(ctx, numWorkers, func(f models.ScannedFile) {
		path, version := f.Path, f.Version
		relPath, _ := utils.SafeRel(s.cfg.ContentDir, path)
		htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

		versionLower := strings.ToLower(version)
		cleanHtmlRelPath := htmlRelPath
		if version != "" {
			cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, versionLower+"/")
		}

		var destPath string
		if version != "" {
			destPath = filepath.Join(s.cfg.OutputDir, version, cleanHtmlRelPath)
		} else {
			destPath = filepath.Join(s.cfg.OutputDir, htmlRelPath)
		}

		// A) Check Cache
		var cachedMeta *cache.PostMeta
		var exists bool
		var err error
		if s.cache != nil {
			cachedMeta, err = s.cache.GetPostByPath(relPath)
			if err == nil && cachedMeta != nil {
				exists = true
			}
		}

		fastBail := !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash == f.BodyHash && cachedMeta.ContentHash == f.FrontmatterHash
		useCache := exists && !shouldForce && fastBail

		var parseRes *ParsedMarkdownResult
		var htmlContent string
		var finalSSRHashes []string

		if useCache {
			cachedHTML, err := s.cache.GetHTMLContent(cachedMeta)
			if err == nil && cachedHTML != nil {
				htmlContent = string(cachedHTML)
				cachedSearch, err := s.cache.GetSearchRecord(cachedMeta.PostID)
				if err == nil && cachedSearch != nil {
					parseRes = &ParsedMarkdownResult{
						MetaData: cachedMeta.Meta, TOC: cachedMeta.TOC,
						FrontmatterHash: cachedMeta.ContentHash, SSRHashes: cachedMeta.SSRInputHashes,
						HasImages: cachedMeta.HasImages, MathExpressions: cachedMeta.MathExpressions,
						SearchRecord: models.PostRecord{
							ID:    uint64(xxh3.HashString(cachedMeta.Link)),
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
					finalSSRHashes = cachedMeta.SSRInputHashes
					s.metrics.IncrementCacheHit()
				} else {
					useCache = false
				}
			} else {
				useCache = false
			}
		}

		// Process math from cached HTML if present
		if useCache && s.diagramAdapter != nil && len(parseRes.MathExpressions) > 0 {
			renderedMath := make(map[string]string)
			for _, expr := range parseRes.MathExpressions {
				if v, ok := s.diagramAdapter.GetLocal(expr.Hash); ok {
					renderedMath[expr.Hash] = v
				}
			}
			if len(renderedMath) > 0 {
				htmlContent = mdParser.ReplaceMathExpressions(htmlContent, parseRes.MathExpressions, renderedMath)
			}
		}

		if !useCache {
			s.metrics.IncrementCacheMiss()
			parseRes, err = ParseMarkdown(ctx, f.Source, path, version, cleanHtmlRelPath, htmlRelPath, s.mdPool, s.cfg, s.nativeRenderer, s.diagramAdapter, &s.mu, f.FrontmatterHash, f.ReadingTime, f.BodyOffset, f.PreParsedMeta)
			if err != nil {
				s.logger.Error("Failed to parse markdown", "path", path, "error", err)
				phase1Mu.Lock()
				phase1Errs = append(phase1Errs, err)
				phase1Mu.Unlock()
				return
			}
			anyPostChanged.Store(true)

			// B) Inline Math Rendering (Batch for this post)
			if len(parseRes.MathExpressions) > 0 {
				cachedSubset := make(map[string]string)
				if s.diagramAdapter != nil {
					for _, e := range parseRes.MathExpressions {
						if v, ok := s.diagramAdapter.GetLocal(e.Hash); ok {
							cachedSubset[e.Hash] = v
						}
					}
				}

				rendered, err := s.nativeRenderer.RenderAllMath(ctx, parseRes.MathExpressions, cachedSubset)
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

				htmlContent = mdParser.ReplaceMathExpressions(parseRes.HTMLContent, parseRes.MathExpressions, rendered)
			} else {
				htmlContent = parseRes.HTMLContent
			}
			finalSSRHashes = parseRes.SSRHashes
		}

		post := parseRes.Post
		if post.Draft && !s.cfg.IncludeDrafts {
			return
		}

		// C) Social Card (Async)
		cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp"))
		cardHash, _ := s.cardHashes.Load(relPath)
		if forceSocialRebuild || cardHash != parseRes.FrontmatterHash {
			cardPool.Submit(socialCardTask{
				path: relPath, relPath: strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
				cardDestPath: cardDestPath, metaData: parseRes.MetaData, frontmatterHash: parseRes.FrontmatterHash,
			})
		} else {
			s.sink.Register(cardDestPath)
		}

		// Collect for Phase 2
		indexedMu.Lock()
		// Assign deterministic ID based on post relative link
		searchRecord := parseRes.SearchRecord
		searchRecord.ID = uint64(xxh3.HashString(searchRecord.Link))
		indexedPosts = append(indexedPosts, models.IndexedPost{
			Record: searchRecord, WordFreqs: parseRes.WordFreqs, DocLen: parseRes.DocLen,
			StemMap: parseRes.StemMap, PositionalIndex: parseRes.PositionalIndex, ByteOffsets: parseRes.ByteOffsets,
		})
		batchMu.Lock()
		allPosts = append(allPosts, post)
		if post.Pinned {
			pinnedPosts = append(pinnedPosts, post)
		}
		for _, tag := range post.Tags {
			k := strings.ToLower(strings.TrimSpace(tag))
			tagMap[k] = append(tagMap[k], post)
		}
		batchMu.Unlock()
		indexedMu.Unlock()

		renderMu.Lock()
		readyToRender = append(readyToRender, renderTask{
			parseRes: parseRes, f: f, htmlContent: htmlContent,
			destPath: destPath, version: version, relPath: relPath, htmlRelPath: htmlRelPath,
		})
		renderMu.Unlock()

		if !useCache && s.cache != nil {
			postID := cache.GeneratePostID("", relPath)
			newMeta := &cache.PostMeta{
				PostID: postID, Path: relPath, ModTime: f.Info.ModTime().Unix(),
				ContentHash: parseRes.FrontmatterHash, BodyHash: f.BodyHash, Title: post.Title, Date: post.DateObj,
				Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
				Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
				Meta: parseRes.MetaData, TOC: parseRes.TOC, Version: version, SSRInputHashes: finalSSRHashes,
				CardHash: parseRes.FrontmatterHash, HasImages: parseRes.HasImages, MathExpressions: parseRes.MathExpressions,
			}
			_ = s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent))
			newSearch := &cache.SearchRecord{
				Title: post.Title, NormalizedTitle: parseRes.SearchRecord.NormalizedTitle,
				BM25Data: parseRes.WordFreqs, DocLen: parseRes.DocLen, Content: parseRes.SearchRecord.Content,
				NormalizedTags: parseRes.SearchRecord.NormalizedTags, StemMap: parseRes.StemMap,
				PositionalIndex: parseRes.PositionalIndex, ByteOffsets: parseRes.ByteOffsets,
			}
			batchMu.Lock()
			newPostsMeta = append(newPostsMeta, newMeta)
			newSearchRecords[postID] = newSearch
			newDeps[postID] = &cache.Dependencies{Tags: post.Tags}
			batchMu.Unlock()
		}
		s.metrics.IncrementPostsProcessed()
	}).WithScheduler(utils.GlobalScheduler, utils.TaskMarkdown)

	parsePool.Start()
	for f := range fileChan {
		parsePool.Submit(f)
	}
	parsePool.Stop()

	if len(phase1Errs) > 0 {
		return nil, phase1Errs[0]
	}

	// --- Phase 1 Complete (Metadata Aggregated) ---

	// Notify build loop that metadata is ready for sitemap, RSS, etc.
	utils.SortPosts(allPosts)
	if s.metadataCallback != nil {
		s.metadataCallback(allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged.Load())
	}

	// Prepare versioned navigation
	postsByVersion := make(map[string][]models.PostMetadata)
	postPosByVersion := make(map[string]map[string]int)
	for _, p := range allPosts {
		postsByVersion[p.Version] = append(postsByVersion[p.Version], p)
	}
	for ver, posts := range postsByVersion {
		utils.SortPosts(posts)
		postPosByVersion[ver] = make(map[string]int)
		for i, p := range posts {
			postPosByVersion[ver][p.Link] = i
		}
	}

	// Phase 2: Rendering (Parallel)
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
	for _, rt := range readyToRender {
		renderPool.Submit(rt)
	}
	renderPool.Stop()

	cardPool.Stop()

	if len(newPostsMeta) > 0 && s.cache != nil {
		s.cacheWg.Add(1)
		go func() {
			defer s.cacheWg.Done()
			_ = s.cache.BatchCommit(newPostsMeta, newSearchRecords, newDeps)
		}()
	}

	return &PostResult{
		AllPosts: allPosts, PinnedPosts: pinnedPosts, TagMap: tagMap,
		IndexedPosts: indexedPosts, AnyPostChanged: anyPostChanged.Load(), Has404: has404,
	}, nil
}
