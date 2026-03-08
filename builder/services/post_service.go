package services

import (
	"context"
	"fmt"
	"html/template"
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

	// Collected social card hashes for batch write after cardPool completes
	cardHashes sync.Map

	// assetsReady is closed when asset building completes. The render phase
	// waits on this channel before reading the Assets map, allowing the
	// parse phase to overlap with asset building for faster cold builds.
	assetsReady <-chan struct{}

	// metadataCallback is invoked when post metadata becomes available
	// (after parse completes, before render starts), allowing site-wide
	// tasks to overlap with the render phase.
	metadataCallback MetadataReadyFunc
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

func (s *postServiceImpl) SetSourceFs(fs afero.Fs) {
	s.sourceFs = fs
}

func (s *postServiceImpl) SetAssetsGate(ch <-chan struct{}) {
	s.assetsReady = ch
}

func (s *postServiceImpl) SetMetadataCallback(fn MetadataReadyFunc) {
	s.metadataCallback = fn
}

func (s *postServiceImpl) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, earlyMetadata *MetadataScannerResult) (*PostResult, error) {
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

    type walkFile struct {
        path    string
        version string
        info    os.FileInfo
    }

    var files []walkFile
    if earlyMetadata != nil && len(earlyMetadata.Files) > 0 {
        // Reuse the scanner's file list to avoid a second walk and extra stats.
        for _, f := range earlyMetadata.Files {
            // Skip 404 here; scanner already marked Has404.
            if strings.Contains(f.Path, "404.md") {
                has404 = has404 || earlyMetadata.Has404
                continue
            }
            files = append(files, walkFile{path: f.Path, version: f.Version, info: f.Info})
        }
        has404 = has404 || earlyMetadata.Has404
    } else {
        if err := afero.Walk(s.sourceFs, s.cfg.ContentDir, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                s.logger.Error("Error walking content directory", "path", path, "error", err)
                return nil
            }
            if !info.IsDir() && strings.HasSuffix(path, ".md") && !strings.Contains(path, "_index.md") {
                if strings.Contains(path, "404.md") {
                    has404 = true
                } else {
                    ver, _ := utils.GetVersionFromPath(path)
                    files = append(files, walkFile{path: path, version: ver, info: info})
                }
            }
            return nil
        }); err != nil {
            s.logger.Error("Failed to walk content directory", "error", err)
        }
    }

	existingFiles := make(map[string]bool)
	for _, f := range files {
		relPath, err := utils.SafeRel(s.cfg.ContentDir, f.path)
		if err != nil {
			s.logger.Error("Failed to get relative path", "file", f.path, "error", err)
			continue
		}
		existingFiles[relPath] = true
	}

	var allMetadataMap sync.Map

	// Phase 0: Load all cached post IDs once, use for both stale-purge and global metadata loading
	if s.cache != nil {
		if lister, ok := s.cache.(interface{ ListAllPosts() ([]string, error) }); ok {
			allCachedIDs, err := lister.ListAllPosts()
			if err != nil {
				s.logger.Error("Failed to list cached posts", "error", err)
			} else {
				// Batch-load all metadata for stale purge + metadata map population
				cachedPosts, _ := s.cache.GetPostsByIDs(allCachedIDs)

				for _, id := range allCachedIDs {
					cp, ok := cachedPosts[id]
					if !ok || cp == nil {
						continue
					}
					// Stale cache purge
					if !existingFiles[cp.Path] {
						s.logger.Info("🗑️ Purging stale cache entry", "path", cp.Path)
						if err := s.cache.DeletePost(id); err != nil {
							s.logger.Error("Failed to delete stale cache entry", "id", id, "error", err)
						}
						continue
					}
					// Phase 0: Populate allMetadataMap for sidebar/neighbor context
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
	}

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

	indexedPosts := make([]models.IndexedPost, len(files))
	var indexedPostIdx int32 = -1

    // renderQueue is built via append to avoid sparse slots when posts are skipped
    renderQueue := make([]RenderContext, 0, len(files))

    // destExistsCache avoids repeated os.Stat calls for the same output path
    var destExistsCache sync.Map

	numWorkers := utils.GetDefaultWorkerCount()
	if s.cfg.ParserWorkers > 0 {
		numWorkers = s.cfg.ParserWorkers
	}

	cardPool := utils.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) {
		s.generateSocialCard(task)
	})
	cardPool.Start()

	s.logger.Info("Parsing posts", "count", len(files))
	parsePhaseName := fmt.Sprintf("Parse %d posts", len(files))
	parseTimer := utils.StartPhase(parsePhaseName)
    type parseTask struct {
        f            walkFile
        versionLower string
    }

    parsePool := utils.NewWorkerPool(ctx, numWorkers, func(pt parseTask) {
        f, versionLower := pt.f, pt.versionLower
		path, version := f.path, f.version

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
		var source []byte
		var bodyHash string
		exists := false

		if s.cache != nil {
			cachedMeta, err = s.cache.GetPostByPath(relPath)
			if err == nil && cachedMeta != nil {
				exists = true
			}
		}

		info := f.info
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
			cachedHash = cachedMeta.CardHash
			if cachedHash == "" {
				// Backward-compat for old cache entries that don't have CardHash in PostMeta.
				cachedHash, _ = s.cache.GetSocialCardHash(relPath)
			}
		}

		var htmlContent string
		var htmlFromCache []byte // set in cache-hit path to avoid string([]byte) round-trip
		var metaData map[string]interface{}
		var post models.PostMetadata
		var searchRecord models.PostRecord
		var wordFreqs map[string]int
		var docLen int
		var toc []models.TOCEntry
		var frontmatterHash string
		var ssrHashes []string
		var offsets map[string][]int

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
			htmlFromCache = cachedHTML
			metaData = cachedMeta.Meta
			frontmatterHash = cachedMeta.ContentHash
			ssrHashes = cachedMeta.SSRInputHashes

			if v, ok := allMetadataMap.Load(cachedMeta.Path); ok {
				if cachedPost, ok := v.(models.PostMetadata); ok {
					post = cachedPost
				}
			}

			toc = cachedMeta.TOC

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
			offsets = cachedSearch.ByteOffsets
		} else {
			s.metrics.IncrementCacheMiss()

        // Copy raw markdown to output for "View Source" feature (reuse already-read buffer)
        if s.cfg.Features.RawMarkdown && len(source) > 0 {
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
		}

		if post.Draft && !s.cfg.IncludeDrafts {
			allMetadataMap.Delete(relPath)
			return
		}

        cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp"))
        if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
            s.logger.Error("Failed to create social card directory", "path", filepath.Dir(cardDestPath), "error", err)
        }

		// Prefer cache hash to avoid repeated fs.Stat on card files.
		cardExists := cachedHash != "" && cachedHash == frontmatterHash && !forceSocialRebuild
		// For clean builds/output-missing, skip expensive per-post stat checks.
		if !outputMissing && !cardExists && !(useCache && cachedHash == "") {
			if info, err := os.Stat(cardDestPath); err == nil && !info.IsDir() {
				if info.ModTime().After(f.info.ModTime()) {
					cardExists = true
				}
			}
		}

		if forceSocialRebuild || (cachedHash != frontmatterHash || !cardExists) {
			if !utils.TestingMode {
				cardPool.Submit(socialCardTask{
					path:            relPath,
					relPath:         strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
					cardDestPath:    cardDestPath,
					metaData:        metaData,
					frontmatterHash: frontmatterHash,
				})
			}
		} else if cardExists {
			if s.cache != nil && cachedHash == "" {
				s.cardHashes.Store(relPath, frontmatterHash)
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
		if outputMissing || !useCache || s.cfg.IsDev {
			willRender = true
		} else if useCache {
            if v, ok := destExistsCache.Load(destPath); ok {
                willRender = !v.(bool)
            } else {
                _, statErr := os.Stat(destPath)
                exists := statErr == nil
                destExistsCache.Store(destPath, exists)
                willRender = !exists
            }
        }

        // Copy raw markdown to output for "View Source" feature (for cached posts too)
        if s.cfg.Features.RawMarkdown {
            mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
            // Avoid extra stat/read: if we already have source bytes, reuse; otherwise read once.
            if willRender || !useCache || outputMissing {
                sourceBytes := source
                if len(sourceBytes) == 0 {
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
                // Cache hit with existing output assumed; just register to prevent cleanup
                s.renderer.RegisterFile(mdDestPath)
            }
        }

        if willRender {
            // Use cached bytes directly to avoid string↔[]byte round-trip
            var contentHTML template.HTML
            if htmlFromCache != nil {
                contentHTML = template.HTML(htmlFromCache)
            } else {
                contentHTML = template.HTML(htmlContent)
            }
            renderQueue = append(renderQueue, RenderContext{
                DestPath: destPath,
                Version:  version,
                Data: models.PageData{
                    Title: post.Title, Description: post.Description, Content: contentHTML,
                    Meta: metaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
                    TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
                    TOC: toc, Config: s.cfg,
                    CurrentVersion: version,
                    IsOutdated:     s.isOutdatedVersion(version),
                    Versions:       s.cfg.GetVersionsMetadata(version, cleanHtmlRelPath),
                    RelativePrefix: utils.GetRelativePrefix(htmlRelPath),
                    ReadingTime:    post.ReadingTime,
                },
            })
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
			ByteOffsets:     offsets,
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
				CardHash:       frontmatterHash,
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
				ByteOffsets:     offsets,
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
    for _, f := range files {
        select {
        case <-ctx.Done():
            break Loop
        default:
            parsePool.Submit(parseTask{f: f, versionLower: strings.ToLower(f.version)})
        }
    }
	parsePool.Stop()
	parseTimer.Stop()
	// NOTE: cardPool is NOT stopped here — it continues generating social cards
	// in parallel with the render phase below. Cards write to VFS/disk independently
	// and the render phase only needs the imagePath URL string (already computed).
	// cardPool.Stop() is called after renderPool.Stop() to overlap both phases.

	// Final Metadata Grouping (merges Cache + Source)
	// Use early metadata from scanner if available, otherwise compute from scratch
	var groupRes *GroupMetadataResult
	if earlyMetadata != nil {
		// Convert early metadata to PostMetadata and build tagMap
		earlyTagMap := make(map[string][]models.PostMetadata)
		earlyPostsByVersion := make(map[string][]models.PostMetadata)

		for tag, lightPosts := range earlyMetadata.TagMap {
			var posts []models.PostMetadata
			for _, lp := range lightPosts {
				if !lp.Draft || s.cfg.IncludeDrafts {
					posts = append(posts, models.PostMetadata{
						Title:       lp.Title,
						Link:        lp.Link,
						Weight:      lp.Weight,
						Version:     lp.Version,
						DateObj:     lp.DateObj,
						Tags:        lp.Tags,
						Pinned:      lp.Pinned,
						Draft:       lp.Draft,
						ReadingTime: lp.ReadingTime,
						Description: lp.Description,
					})
				}
			}
			if len(posts) > 0 {
				earlyTagMap[tag] = posts
			}
		}

		for ver, lightPosts := range earlyMetadata.PostsByVersion {
			var posts []models.PostMetadata
			for _, lp := range lightPosts {
				if !lp.Draft || s.cfg.IncludeDrafts {
					posts = append(posts, models.PostMetadata{
						Title:       lp.Title,
						Link:        lp.Link,
						Weight:      lp.Weight,
						Version:     lp.Version,
						DateObj:     lp.DateObj,
						Tags:        lp.Tags,
						Pinned:      lp.Pinned,
						Draft:       lp.Draft,
						ReadingTime: lp.ReadingTime,
						Description: lp.Description,
					})
				}
			}
			if len(posts) > 0 {
				earlyPostsByVersion[ver] = posts
			}
		}

		// Build allPosts and pinnedPosts from early metadata
		var earlyAllPosts []models.PostMetadata
		var earlyPinnedPosts []models.PostMetadata
		for _, posts := range earlyPostsByVersion {
			for _, p := range posts {
				isLatestOrUnversioned := p.Version == ""
				if len(s.cfg.Versions) > 0 {
					for _, v := range s.cfg.Versions {
						if v.IsLatest && p.Version == v.Name {
							isLatestOrUnversioned = true
							break
						}
					}
				}
				if isLatestOrUnversioned {
					if p.Pinned {
						earlyPinnedPosts = append(earlyPinnedPosts, p)
					} else {
						earlyAllPosts = append(earlyAllPosts, p)
					}
				}
			}
		}

		groupRes = &GroupMetadataResult{
			AllPosts:       earlyAllPosts,
			PinnedPosts:    earlyPinnedPosts,
			TagMap:         earlyTagMap,
			PostsByVersion: earlyPostsByVersion,
		}
	} else {
		groupRes = GroupMetadata(s.cfg, &allMetadataMap)
	}
	allPosts = groupRes.AllPosts
	pinnedPosts = groupRes.PinnedPosts
	tagMap = groupRes.TagMap
	postsByVersion = groupRes.PostsByVersion

	// Sort posts for consistent ordering — needed by sitemap, RSS, pagination.
	// Safe to do here because parse phase is complete and allPosts/pinnedPosts are final.
	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	// Compact indexedPosts early — parse is complete so indexedPostIdx is final.
	// This is needed by the search index generator which may start via the callback below.
	finalCount := int(indexedPostIdx + 1)
	finalIndexedPosts := make([]models.IndexedPost, 0, finalCount)
	for i := 0; i < finalCount; i++ {
		if indexedPosts[i].Record.Title != "" {
			indexedPosts[i].Record.ID = len(finalIndexedPosts)
			finalIndexedPosts = append(finalIndexedPosts, indexedPosts[i])
		}
	}

	// Notify build.go that metadata is ready — site-wide tasks (sitemap, RSS,
	// search, pagination, tags, PWA) can now start in parallel with the render phase.
	if s.metadataCallback != nil {
		s.metadataCallback(allPosts, pinnedPosts, tagMap, finalIndexedPosts, anyPostChanged.Load())
	}

	siteTrees := BuildSiteTrees(postsByVersion)
	sidebarCache := make(map[string]template.HTML)
	for version, tree := range siteTrees {
		sidebarCache[version] = s.renderer.RenderSidebar(tree)
	}

    renderPool := utils.NewWorkerPool(ctx, numWorkers, func(t RenderContext) {
        t.Data.SiteTree = siteTrees[t.Version]
        t.Data.SidebarHTML = sidebarCache[t.Version]
        s.renderer.RenderPage(t.DestPath, t.Data)
    })
	renderPool.Start()

	s.logger.Info("Rendering pages", "count", len(renderQueue))
	renderPhaseName := fmt.Sprintf("Render %d pages", len(renderQueue))
	renderTimer := utils.StartPhase(renderPhaseName)

	// Only block on assets when they are not already available (helps incremental builds).
	assets := s.renderer.GetAssets()
	if s.assetsReady != nil && len(assets) == 0 {
		<-s.assetsReady
		assets = s.renderer.GetAssets()
	}

	// Pre-index post positions for O(1) neighbor lookup
	postPosByVersion := make(map[string]map[string]int)
	for version, posts := range postsByVersion {
		postPosByVersion[version] = make(map[string]int, len(posts))
		for i, p := range posts {
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

	// Now wait for social card generation to complete (overlapped with render phase above)
	cardPool.Stop()

	// Batch-flush collected social card hashes in a single BoltDB transaction
	if s.cache != nil {
		pendingHashes := make(map[string]string)
		s.cardHashes.Range(func(key, value any) bool {
			pendingHashes[key.(string)] = value.(string)
			return true
		})
		if len(pendingHashes) > 0 {
			if err := s.cache.BatchSetSocialCardHashes(pendingHashes); err != nil {
				s.logger.Warn("Failed to batch-set social card hashes", "error", err)
			}
		}
	}

	if s.cache != nil && len(newPostsMeta) > 0 {
		cacheCommitTimer := utils.StartPhase("Cache commit")
		err := s.cache.BatchCommit(newPostsMeta, newSearchRecords, newDeps)
		cacheCommitTimer.Stop()
		if err != nil {
			s.logger.Warn("Failed to commit cache batch", "error", err)
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
