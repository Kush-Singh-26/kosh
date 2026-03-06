package services

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

type postServiceImpl struct {
	cfg            *config.Config
	cache          CacheService
	renderer       RenderService
	logger         *slog.Logger
	metrics        *metrics.BuildMetrics
	mdPool         *sync.Pool // Replaces s.md and prevents contention via thread-local checkout
	nativeRenderer *native.Renderer
	sourceFs       afero.Fs
	sink           utils.ArtifactSink
	diagramAdapter *cache.DiagramCacheAdapter

	// Mutex for D2/Math rendering safety if needed
	mu sync.Mutex
}

func NewPostService(
	cfg *config.Config,
	cacheSvc CacheService,
	renderer RenderService,
	logger *slog.Logger,
	metrics *metrics.BuildMetrics,
	mdPool *sync.Pool,
	nativeRenderer *native.Renderer,
	sourceFs afero.Fs,
	sink utils.ArtifactSink,
	diagramAdapter *cache.DiagramCacheAdapter,
) PostService {
	return &postServiceImpl{
		cfg:            cfg,
		cache:          cacheSvc,
		renderer:       renderer,
		logger:         logger,
		metrics:        metrics,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
		sourceFs:       sourceFs,
		sink:           sink,
		diagramAdapter: diagramAdapter,
	}
}

func (s *postServiceImpl) SetSink(sink utils.ArtifactSink) {
	s.sink = sink
}

func (s *postServiceImpl) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool) (*PostResult, error) {
	if os.Getenv("KOSH_FORCE_REBUILD") == "1" {
		shouldForce = true
	}

	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		tagMap         map[string][]models.PostMetadata
		postsByVersion map[string][]models.PostMetadata
		has404         bool
		anyPostChanged atomic.Bool
		processedCount int32
	)

	var files []string
	var fileVersions []string
	if err := afero.Walk(s.sourceFs, s.cfg.ContentDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			s.logger.Error("Error walking content directory", "path", path, "error", err)
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") && !strings.Contains(path, "_index.md") {
			if strings.Contains(path, "404.md") {
				has404 = true
			} else {
				ver, _ := utils.GetVersionFromPath(path)
				files = append(files, path)
				fileVersions = append(fileVersions, ver)
			}
		}
		return nil
	}); err != nil {
		s.logger.Error("Failed to walk content directory", "error", err)
	}

	existingFiles := make(map[string]bool)
	for _, f := range files {
		relPath, err := utils.SafeRel(s.cfg.ContentDir, f)
		if err != nil {
			s.logger.Error("Failed to get relative path", "file", f, "error", err)
			continue
		}
		existingFiles[relPath] = true
	}

	if s.cache != nil {
		if lister, ok := s.cache.(interface{ ListAllPosts() ([]string, error) }); ok {
			ids, err := lister.ListAllPosts()
			if err != nil {
				s.logger.Error("Failed to list cached posts", "error", err)
			} else {
				for _, id := range ids {
					meta, err := s.cache.GetPost(id)
					if err != nil || meta == nil {
						continue
					}
					if !existingFiles[meta.Path] {
						s.logger.Info("🗑️ Purging stale cache entry", "path", meta.Path)
						if err := s.cache.DeletePost(id); err != nil {
							s.logger.Error("Failed to delete stale cache entry", "id", id, "error", err)
						}
					}
				}
			}
		}
	}

	var allMetadataMap sync.Map

	var (
		batchMu          sync.Mutex
		newPostsMeta     []*cache.PostMeta
		newSearchRecords = make(map[string]*cache.SearchRecord)
		newDeps          = make(map[string]*cache.Dependencies)
	)

	type RenderContext struct {
		DestPath string
		Data     models.PageData
		Version  string
	}

	// Pre-allocate indexed posts slice and use atomic index for lock-free writes
	indexedPosts := make([]models.IndexedPost, len(files))
	var indexedPostIdx int32 = -1 // Start at -1 so first AddInt32 returns 0

	renderQueue := make([]RenderContext, len(files))

	numWorkers := utils.GetDefaultWorkerCount()
	if s.cfg.ParserWorkers > 0 {
		numWorkers = s.cfg.ParserWorkers
	}

	cardPool := utils.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) {
		s.generateSocialCard(task)
	})
	cardPool.Start()

	// Phase 0: Load global metadata from cache for complete sidebar/neighbor context
	if s.cache != nil {
		if lister, ok := s.cache.(interface{ ListAllPosts() ([]string, error) }); ok {
			ids, _ := lister.ListAllPosts()
			cachedPosts, _ := s.cache.GetPostsByIDs(ids)
			for _, cp := range cachedPosts {
				// Reconstruct correct link for the current BaseURL
				htmlRelPath := strings.ToLower(strings.Replace(cp.Path, ".md", ".html", 1))
				cleanHtmlRelPath := htmlRelPath
				if cp.Version != "" {
					cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(cp.Version)+"/")
				}
				regeneratedLink := utils.BuildURL(s.cfg.BaseURL, cp.Version, cleanHtmlRelPath)

				allMetadataMap.Store(cp.Path, models.PostMetadata{
					Title: cp.Title, Link: regeneratedLink, Weight: cp.Weight, Version: cp.Version,
					DateObj: cp.Date, ReadingTime: cp.ReadingTime, Description: cp.Description,
					Tags: cp.Tags, Pinned: cp.Pinned, Draft: cp.Draft,
				})
			}
		}
	}

	s.logger.Info("Parsing posts", "count", len(files))
	parsePhaseName := fmt.Sprintf("Parse %d posts", len(files))
	parseTimer := utils.StartPhase(parsePhaseName)
	parsePool := utils.NewWorkerPool(ctx, numWorkers, func(pt struct {
		idx          int
		path         string
		version      string
		versionLower string
	}) {
		idx, path, version, versionLower := pt.idx, pt.path, pt.version, pt.versionLower

		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("PANIC recovered while processing post",
					"path", path,
					"error", r,
					"stack", string(debug.Stack()))
			}
		}()

		relPath, _ := utils.SafeRel(s.cfg.ContentDir, path)
		htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

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

		// 1. Resolve from Cache
		var cachedMeta *cache.PostMeta
		var cachedSearch *cache.SearchRecord
		var cachedHTML []byte
		var err error
		var info os.FileInfo
		var source []byte
		var bodyHash string
		exists := false

		if s.cache != nil {
			cachedMeta, err = s.cache.GetPostByPath(relPath)
			if err == nil && cachedMeta != nil {
				exists = true
			}
		}

		info, err = s.sourceFs.Stat(path)
		if err != nil {
			s.logger.Error("Failed to stat file", "path", path, "error", err)
			return
		}
		if info.Size() > utils.MaxFileSize {
			s.logger.Warn("File exceeds size limit, skipping", "path", path, "size", info.Size(), "limit", utils.MaxFileSize)
			return
		}

		// Fast-bail: if ModTime exactly matches and we have a valid body hash, skip computing hashes.
		// ONLY do this if not forcing rebuild. If fastBail is true, useCache becomes true.
		fastBail := !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash != "" && info != nil && cachedMeta.ModTime == info.ModTime().Unix()

		if !fastBail {
			// Always read source to compute body hash (CRITICAL for cache validity)
			source, err = afero.ReadFile(s.sourceFs, path)
			if err != nil {
				s.logger.Error("Failed to read file", "path", path, "error", err)
				return
			}
			bodyHash = utils.GetBodyHash(source)

			// Invalidate cache if body content changed (regardless of ModTime)
			// Also invalidate if bodyHash is empty (old cache entry without body hash)
			if exists && cachedMeta != nil {
				if cachedMeta.BodyHash == "" || cachedMeta.BodyHash != bodyHash {
					exists = false
				}
			}

			// Invalidate cache if frontmatter changed (compute hash from raw source without full parsing)
			if exists && cachedMeta != nil {
				currentFrontmatterHash, _ := utils.GetFrontmatterHashFromSource(source)
				if currentFrontmatterHash == "" || currentFrontmatterHash != cachedMeta.ContentHash {
					exists = false
				}
			}
		}

		useCache := exists && !shouldForce

		var cachedHash string
		if s.cache != nil && !useCache {
			cachedHash, _ = s.cache.GetSocialCardHash(relPath)
		} else if useCache && cachedMeta != nil {
			cachedHash = cachedMeta.ContentHash
		}

		var htmlContent string
		var metaData map[string]interface{}
		var post models.PostMetadata
		var searchRecord models.PostRecord
		var wordFreqs map[string]int
		var docLen int
		var toc []models.TOCEntry
		var frontmatterHash string
		var ssrHashes []string

		if useCache {
			cachedHTML, err = s.cache.GetHTMLContent(cachedMeta)
			if err != nil || cachedHTML == nil {
				useCache = false
			} else {
				cachedSearch, err = s.cache.GetSearchRecord(cachedMeta.PostID)
				if err != nil || cachedSearch == nil {
					useCache = false
				}
			}
		}

		if !useCache && len(source) == 0 {
			source, err = afero.ReadFile(s.sourceFs, path)
			if err != nil {
				s.logger.Error("Failed to read file", "path", path, "error", err)
				return
			}
			if bodyHash == "" {
				bodyHash = utils.GetBodyHash(source)
			}
		}

		var stemMap map[string]string
		var posIndex map[string][]int
		if useCache {
			s.metrics.IncrementCacheHit()
			htmlContent = string(cachedHTML)
			metaData = cachedMeta.Meta
			frontmatterHash = cachedMeta.ContentHash
			ssrHashes = cachedMeta.SSRInputHashes

			if v, ok := allMetadataMap.Load(cachedMeta.Path); ok {
				if cachedPost, ok := v.(models.PostMetadata); ok {
					post = cachedPost
				}
			}

			for _, t := range cachedMeta.TOC {
				toc = append(toc, models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level})
			}

			searchRecord = models.PostRecord{
				Title:           cachedSearch.Title,
				NormalizedTitle: cachedSearch.NormalizedTitle,
				Link:            htmlRelPath,
				Description:     cachedMeta.Description,
				Tags:            cachedMeta.Tags,
				NormalizedTags:  cachedSearch.NormalizedTags,
				Content:         cachedSearch.Content,
				Version:         cachedMeta.Version,
			}
			docLen = cachedSearch.DocLen
			wordFreqs = cachedSearch.BM25Data
			stemMap = cachedSearch.StemMap
			posIndex = cachedSearch.PositionalIndex
		} else {
			s.metrics.IncrementCacheMiss()

			// Copy raw markdown to output for "View Source" feature
			if s.cfg.Features.RawMarkdown {
				// Use filepath to handle OS-specific path separators correctly
				mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
				if err := s.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
					s.logger.Error("Failed to create markdown directory", "path", filepath.Dir(mdDestPath), "error", err)
				}
				if err := s.sink.WriteFile(mdDestPath, source); err != nil {
					s.logger.Error("Failed to write markdown file", "path", mdDestPath, "error", err)
				} else {
					s.renderer.RegisterFile(mdDestPath)
				}
			}

			parseRes, parseErr := ParseMarkdown(
				ctx,
				source,
				path,
				version,
				cleanHtmlRelPath,
				htmlRelPath,
				s.mdPool,
				s.cfg,
				s.nativeRenderer,
				s.diagramAdapter,
				&s.mu,
			)

			if parseErr != nil {
				s.logger.Error("Failed to parse markdown", "path", path, "error", parseErr)
				return
			}

			htmlContent = parseRes.HTMLContent
			metaData = parseRes.MetaData
			post = parseRes.Post
			searchRecord = parseRes.SearchRecord
			toc = parseRes.TOC
			frontmatterHash = parseRes.FrontmatterHash
			ssrHashes = parseRes.SSRHashes
			wordFreqs = parseRes.WordFreqs
			docLen = parseRes.DocLen
			stemMap = parseRes.StemMap
			posIndex = parseRes.PositionalIndex

			indexedPost := models.IndexedPost{
				Record:          searchRecord,
				WordFreqs:       wordFreqs,
				DocLen:          docLen,
				StemMap:         stemMap,
				PositionalIndex: posIndex,
			}
			// (Optimization check: we'll assign it to the slice later)
			_ = indexedPost
		}

		if post.Draft && !s.cfg.IncludeDrafts {
			allMetadataMap.Delete(relPath)
			return
		}

		cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp"))
		if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
			s.logger.Error("Failed to create social card directory", "path", filepath.Dir(cardDestPath), "error", err)
		}

		// Check if card exists in OS filesystem (bypass VFS)
		cardExists := false
		if info, err := os.Stat(cardDestPath); err == nil && !info.IsDir() {
			if sourceInfo, err := s.sourceFs.Stat(path); err == nil {
				if info.ModTime().After(sourceInfo.ModTime()) {
					cardExists = true
				}
			}
		}

		if forceSocialRebuild || (cachedHash != frontmatterHash || !cardExists) {
			cardPool.Submit(socialCardTask{
				path:            relPath,
				relPath:         strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
				cardDestPath:    cardDestPath,
				metaData:        metaData,
				frontmatterHash: frontmatterHash,
			})
		} else if cardExists {
			if s.cache != nil && cachedHash == "" {
				if err := s.cache.SetSocialCardHash(relPath, frontmatterHash); err != nil {
					s.logger.Error("Failed to set social card hash", "path", relPath, "error", err)
				}
			}
		}

		imagePath := s.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
		if img, ok := metaData["image"].(string); ok {
			if s.cfg.CompressImages && !strings.HasPrefix(img, "http") {
				ext := filepath.Ext(img)
				if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
					img = img[:len(img)-len(ext)] + ".webp"
				}
			}
			imagePath = s.cfg.BaseURL + img
		}

		willRender := false
		if outputMissing || !useCache {
			willRender = true
		} else {
			// useCache is true, only render if output file doesn't exist
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				willRender = true
			}
		}

		// Copy raw markdown to output for "View Source" feature (for cached posts too)
		if s.cfg.Features.RawMarkdown {
			mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
			if _, err := os.Stat(mdDestPath); os.IsNotExist(err) {
				var sourceBytes []byte
				var err error
				if len(source) > 0 {
					sourceBytes = source
				} else {
					sourceBytes, err = afero.ReadFile(s.sourceFs, path)
				}

				if err != nil {
					s.logger.Error("Failed to read source file for raw markdown", "path", path, "error", err)
				} else if len(sourceBytes) > 0 {
					if err := s.sink.MkdirAll(filepath.Dir(mdDestPath)); err != nil {
						s.logger.Error("Failed to create markdown directory", "path", filepath.Dir(mdDestPath), "error", err)
					}
					if err := s.sink.WriteFile(mdDestPath, sourceBytes); err != nil {
						s.logger.Error("Failed to write raw markdown file", "path", mdDestPath, "error", err)
					} else {
						s.renderer.RegisterFile(mdDestPath)
					}
				}
			} else {
				s.renderer.RegisterFile(mdDestPath)
			}
		}

		if willRender {
			renderQueue[idx] = RenderContext{
				DestPath: destPath,
				Version:  version,
				Data: models.PageData{
					Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
					Meta: metaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
					TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
					TOC: toc, Config: s.cfg,
					CurrentVersion: version,
					IsOutdated:     s.isOutdatedVersion(version),
					Versions:       s.cfg.GetVersionsMetadata(version, cleanHtmlRelPath),
					RelativePrefix: utils.GetRelativePrefix(htmlRelPath),
				},
			}
			anyPostChanged.Store(true)
		} else {
			// Even if not re-rendering, register the file so it's not cleaned up as orphan
			s.renderer.RegisterFile(destPath)
		}

		// Use sync.Map for metadata (optimization: lock-free concurrent access)
		allMetadataMap.Store(relPath, post)

		// Lock-free indexed post assignment using atomic index
		id := int(atomic.AddInt32(&indexedPostIdx, 1))
		searchRecord.ID = id
		indexedPosts[id] = models.IndexedPost{
			Record:          searchRecord,
			WordFreqs:       wordFreqs,
			DocLen:          docLen,
			StemMap:         stemMap, // Passed from either useCache or fresh analyze
			PositionalIndex: posIndex,
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !useCache && s.cache != nil {
			postID := cache.GeneratePostID("", relPath)
			newMeta := &cache.PostMeta{
				PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
				ContentHash: frontmatterHash, BodyHash: bodyHash, Title: post.Title, Date: post.DateObj,
				Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
				Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
				Meta: metaData, TOC: toc, Version: version,
				SSRInputHashes: ssrHashes,
			}
			if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
				s.logger.Error("Failed to store HTML in cache", "path", relPath, "error", err)
			}
			newSearch := &cache.SearchRecord{
				Title:           post.Title,
				NormalizedTitle: searchRecord.NormalizedTitle,
				BM25Data:        wordFreqs,
				DocLen:          docLen,
				Content:         searchRecord.Content,
				NormalizedTags:  searchRecord.NormalizedTags,
				StemMap:         stemMap,
				PositionalIndex: posIndex,
			}
			newDep := &cache.Dependencies{Tags: post.Tags}

			batchMu.Lock()
			newPostsMeta = append(newPostsMeta, newMeta)
			newSearchRecords[postID] = newSearch
			newDeps[postID] = newDep
			batchMu.Unlock()
		}

		s.metrics.IncrementPostsProcessed()
		_ = atomic.AddInt32(&processedCount, 1)
	})
	parsePool.Start()

Loop:
	for i, path := range files {
		select {
		case <-ctx.Done():
			break Loop
		default:
			parsePool.Submit(struct {
				idx          int
				path         string
				version      string
				versionLower string
			}{i, path, fileVersions[i], strings.ToLower(fileVersions[i])})
		}
	}
	parsePool.Stop()
	parseTimer.Stop()
	cardPool.Stop() // Wait for all social card generation to complete

	// Final Metadata Grouping (merges Cache + Source)
	groupRes := GroupMetadata(s.cfg, &allMetadataMap)
	allPosts = groupRes.AllPosts
	pinnedPosts = groupRes.PinnedPosts
	tagMap = groupRes.TagMap
	postsByVersion = groupRes.PostsByVersion

	siteTrees := BuildSiteTrees(postsByVersion)

	renderPool := utils.NewWorkerPool(ctx, numWorkers, func(t RenderContext) {
		t.Data.SiteTree = siteTrees[t.Version]
		s.renderer.RenderPage(t.DestPath, t.Data)
	})
	renderPool.Start()

	s.logger.Info("Rendering pages", "count", len(renderQueue))
	renderPhaseName := fmt.Sprintf("Render %d pages", len(renderQueue))
	renderTimer := utils.StartPhase(renderPhaseName)

	assets := s.renderer.GetAssets()

	postIndexByVersion := make(map[string]map[string]models.PostMetadata)
	// Pre-index post positions for O(1) neighbor lookup
	postPosByVersion := make(map[string]map[string]int)
	for version, posts := range postsByVersion {
		postIndexByVersion[version] = make(map[string]models.PostMetadata, len(posts))
		postPosByVersion[version] = make(map[string]int, len(posts))
		for i, p := range posts {
			postIndexByVersion[version][p.Link] = p
			postPosByVersion[version][p.Link] = i
		}
	}

	for i := range renderQueue {
		task := &renderQueue[i]
		if task.DestPath == "" {
			continue
		}

		// Inject neighbors (Prev/Next) using O(1) lookup
		versionPosts := postsByVersion[task.Version]
		var prev, next *models.NavPage

		if posIndex, ok := postPosByVersion[task.Version]; ok {
			if idx, ok := posIndex[task.Data.Permalink]; ok {
				// Previous post
				if idx > 0 {
					p := versionPosts[idx-1]
					prev = &models.NavPage{Title: p.Title, Link: p.Link}
				}
				// Next post
				if idx < len(versionPosts)-1 {
					n := versionPosts[idx+1]
					next = &models.NavPage{Title: n.Title, Link: n.Link}
				}
			}
		}

		task.Data.PrevPage = prev
		task.Data.NextPage = next
		task.Data.Assets = assets

		renderPool.Submit(*task)
	}
	renderPool.Stop()
	renderTimer.Stop()

	if s.cache != nil && len(newPostsMeta) > 0 {
		if err := s.cache.BatchCommit(newPostsMeta, newSearchRecords, newDeps); err != nil {
			s.logger.Warn("Failed to commit cache batch", "error", err)
		}
	}

	// Sort posts to ensure consistent ordering
	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	// Compact indexedPosts to remove zero-value entries from skipped files (drafts, oversized)
	finalCount := int(indexedPostIdx + 1)
	finalIndexedPosts := make([]models.IndexedPost, 0, finalCount)
	for i := 0; i < finalCount; i++ {
		if indexedPosts[i].Record.Title != "" {
			indexedPosts[i].Record.ID = len(finalIndexedPosts)
			finalIndexedPosts = append(finalIndexedPosts, indexedPosts[i])
		}
	}

	return &PostResult{
		AllPosts:       allPosts,
		PinnedPosts:    pinnedPosts,
		TagMap:         tagMap,
		IndexedPosts:   finalIndexedPosts,
		AnyPostChanged: anyPostChanged.Load(),
		Has404:         has404,
	}, nil
}
