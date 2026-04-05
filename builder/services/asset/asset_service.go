package asset

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// Option configures optional parameters for AssetService
type Option func(*assetService)

// WithMetrics sets the build metrics collector
func WithMetrics(m *metrics.BuildMetrics) Option {
	return func(s *assetService) { s.metrics = m }
}

// WithAssetsReadySignal sets the channel signaled when assets are ready
func WithAssetsReadySignal(ch chan struct{}) Option {
	return func(s *assetService) { s.assetsReady = ch }
}

// WithContentAssetsChannel sets the channel for content asset notifications
func WithContentAssetsChannel(ch <-chan []models.ScannedAsset) Option {
	return func(s *assetService) { s.contentAssetsChan = ch }
}

// assetService implements AssetService
type assetService struct {
	ctx               *buildCtx.BuildContext
	sourceFs          afero.Fs
	sink              fspkg.ArtifactSink
	cfg               *config.Config
	renderer          render.Service
	logger            *slog.Logger
	metrics           *metrics.BuildMetrics
	contentAssetsChan <-chan []models.ScannedAsset
	assetsReady       chan struct{}
	discoveryReady    chan struct{}
}

func NewService(deps Dependencies, opts ...Option) Service {
	s := &assetService{
		ctx:      deps.Ctx,
		sourceFs: deps.SourceFs,
		sink:     deps.Sink,
		cfg:      deps.Cfg,
		renderer: deps.Renderer,
		logger:   deps.Logger,
		metrics:  deps.Metrics,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *assetService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

func (s *assetService) SetMetrics(m *metrics.BuildMetrics)    { s.metrics = m }
func (s *assetService) SetAssetsReadySignal(ch chan struct{}) { s.assetsReady = ch }
func (s *assetService) SetDiscoveryReady(ch chan struct{})    { s.discoveryReady = ch }
func (s *assetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	s.contentAssetsChan = ch
}

func (s *assetService) DiscoveryReady() <-chan struct{} {
	return s.discoveryReady
}

type assetTask struct {
	srcPath string
	relPath string
	info    fs.FileInfo
}

type imageCopyTask struct {
	task assetTask
	opts assets.ProcessImageOptions
}

func isWebPCandidate(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

// syncStaticAssets discovers and copies all static assets to the sink synchronously.
// For images, it performs cache-lookup and copies cache-hit images immediately.
// For cache-miss images, it returns them in imageQueue for caller to process.
// This method is used by both Build() (in background) and BuildForAssetChange() (inline).
func (s *assetService) syncStaticAssets(ctx context.Context, bgCtx context.Context, skipImages bool) (imageQueue []imageCopyTask, err error) {
	themeDir := s.cfg.StaticDir
	if themeDir == "" {
		themeDir = "themes/blog/static"
	}
	siteStaticDir := "static"
	if s.cfg.SiteRoot != "" {
		siteStaticDir = filepath.Join(s.cfg.SiteRoot, "static")
	}
	debugAssets := os.Getenv("KOSH_DEBUG_ASSETS") == "1"
	var siteFiles, themeFiles, siteEnqueued, themeEnqueued int64
	var relErrs int64
	var siteSamples, themeSamples []string
	var sampleMu sync.Mutex

	numWorkers := s.cfg.ImageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	assetChan := make(chan assetTask, 512)
	var seen sync.Map

	copyGroup, copyCtx := errgroup.WithContext(ctx)
	copyGroup.SetLimit(256)

	workerWg := sync.WaitGroup{}
	workerWg.Add(1)
	go func() {
		defer workerWg.Done()
		for task := range assetChan {
			t := task
			copyGroup.Go(func() error {
				dst := filepath.Join(s.cfg.OutputDir, t.relPath)
				opts := assets.CopyOptions{
					Compress:     s.cfg.CompressImages,
					MinifySVGs:   s.cfg.MinifySVGs,
					KeepOriginal: false,
					CacheDir:     s.cfg.CacheDir + "/images",
					WebPQuality:  s.cfg.WebPQuality,
					Metrics:      s.metrics,
					OnWrite:      s.renderer.RegisterFile,
					ImageWorkers: s.cfg.ImageWorkers,
				}
				return assets.CopyFileWithOptionalImageProcessing(assets.ProcessImageOptions{
					Ctx:     copyCtx,
					SrcFs:   s.sourceFs,
					Sink:    s.sink,
					SrcPath: t.srcPath,
					DstPath: dst,
					RelPath: t.relPath,
					SrcInfo: t.info,
					Opts:    opts,
					Scheduler: func() scheduler.BuildScheduler {
						if s.ctx != nil {
							return s.ctx.Scheduler
						}
						return nil
					}(),
				})
			})
		}
	}()

	walkerWg := sync.WaitGroup{}
	discoveryGroup, dCtx := errgroup.WithContext(ctx)
	walkConcurrency := max(numWorkers/2, 4)

	var imageQueueMu sync.Mutex

	enqueue := func(t assetTask) {
		if s.cfg.CompressImages && isWebPCandidate(t.srcPath) {
			dst := filepath.Join(s.cfg.OutputDir, t.relPath)
			dstWebp := dst[:len(dst)-len(filepath.Ext(dst))] + ".webp"

			// Optimization: If skipImages is true and we are in dev mode (direct-to-output),
			// check if the destination already exists. If it does, we just register it
			// so that it doesn't get cleaned up as an orphan.
			if skipImages && s.cfg.IsDev {
				if _, err := s.sink.Stat(dstWebp); err == nil {
					// Register it in the converted-images map so HTML rewrites work
					relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(t.relPath), "/")
					relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
					assets.RecordConvertedImage(relSrc, relDst)
					assets.RecordConvertedImage(strings.TrimPrefix(relSrc, "/"), relDst)

					// Register it with the sink so it isn't cleaned up by cleanupOrphans
					s.sink.Register(dstWebp)
					if s.renderer != nil {
						s.renderer.RegisterFile(dstWebp)
					}
					return
				}
			}

			err := assets.CopyFromDiskCache(s.sourceFs, s.sink, t.relPath, t.srcPath, dstWebp,
				s.cfg.CacheDir+"/images", t.info, s.metrics, s.renderer.RegisterFile, s.cfg.IsDev || s.cfg.Features.RawMarkdown, skipImages)
			if err == nil {
				return
			}
			if !errors.Is(err, assets.ErrCacheMiss) {
				s.logger.Warn("Disk cache lookup failed", "path", t.srcPath, "error", err)
			}
			imgOpts := assets.ProcessImageOptions{
				Ctx:     bgCtx,
				SrcFs:   s.sourceFs,
				Sink:    s.sink,
				SrcPath: t.srcPath,
				DstPath: dstWebp,
				RelPath: t.relPath,
				SrcInfo: t.info,
				Opts: assets.CopyOptions{
					Compress:     s.cfg.CompressImages,
					MinifySVGs:   s.cfg.MinifySVGs,
					KeepOriginal: false,
					CacheDir:     s.cfg.CacheDir + "/images",
					WebPQuality:  s.cfg.WebPQuality,
					Metrics:      s.metrics,
					OnWrite:      s.renderer.RegisterFile,
					ImageWorkers: s.cfg.ImageWorkers,
					Scheduler: func() scheduler.BuildScheduler {
						if s.ctx != nil {
							return s.ctx.Scheduler
						}
						return nil
					}(),
				},
			}
			imageQueueMu.Lock()
			imageQueue = append(imageQueue, imageCopyTask{task: t, opts: imgOpts})
			imageQueueMu.Unlock()
			return
		}
		assetChan <- t
	}

	walkFunc := func(ctx context.Context, dir, label string, isSite bool) error {
		exists, _ := afero.Exists(s.sourceFs, dir)
		if !exists {
			return nil
		}
		walkerWg.Add(1)
		go func() {
			defer walkerWg.Done()
			_ = fspkg.ParallelWalk(ctx, s.sourceFs, dir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if debugAssets {
					if isSite {
						atomic.AddInt64(&siteFiles, 1)
					} else {
						atomic.AddInt64(&themeFiles, 1)
					}
				}
				if filepath.Base(path) == "search.wasm" {
					return nil
				}
				rel, relErr := fspkg.SafeRel(dir, path)
				if relErr != nil || rel == "" {
					baseNorm := fspkg.NormalizePath(dir)
					pathNorm := fspkg.NormalizePath(path)
					if !fspkg.IsPathInOrSame(pathNorm, baseNorm) {
						if debugAssets {
							atomic.AddInt64(&relErrs, 1)
						}
						return nil
					}
					rel = strings.TrimPrefix(pathNorm, baseNorm)
					rel = strings.TrimPrefix(rel, "/")
					if rel == "" {
						if debugAssets {
							atomic.AddInt64(&relErrs, 1)
						}
						return nil
					}
				}
				fullRel := "static/" + rel
				if _, loaded := seen.LoadOrStore(fullRel, true); !loaded {
					if debugAssets {
						if isSite {
							atomic.AddInt64(&siteEnqueued, 1)
						} else {
							atomic.AddInt64(&themeEnqueued, 1)
						}
						sampleMu.Lock()
						if isSite && len(siteSamples) < 5 {
							siteSamples = append(siteSamples, fullRel)
						} else if !isSite && len(themeSamples) < 5 {
							themeSamples = append(themeSamples, fullRel)
						}
						sampleMu.Unlock()
					}
					enqueue(assetTask{srcPath: path, relPath: fullRel, info: info})
				}
				return nil
			})
		}()
		return nil
	}

	themeDirNorm := fspkg.NormalizePath(themeDir)
	siteStaticNorm := fspkg.NormalizePath(siteStaticDir)
	sameStatic := themeDirNorm == siteStaticNorm
	if runtime.GOOS == "windows" {
		sameStatic = strings.EqualFold(themeDirNorm, siteStaticNorm)
	}

	if !sameStatic {
		// Use parent ctx, not dCtx: walkFunc spawns async goroutines that outlive
		// the discoveryGroup. dCtx is cancelled when Wait() returns, which would
		// abort the still-running ParallelWalk.
		discoveryGroup.Go(func() error { return walkFunc(ctx, siteStaticDir, "site", true) })
	}

	discoveryGroup.Go(func() error { return walkFunc(ctx, themeDir, "theme", false) })

	if s.contentAssetsChan != nil {
		discoveryGroup.Go(func() error {
			select {
			case contentAssets, ok := <-s.contentAssetsChan:
				if ok && contentAssets != nil {
					for _, a := range contentAssets {
						rel, _ := fspkg.SafeRel(s.cfg.ContentDir, a.Path)
						if _, loaded := seen.LoadOrStore(rel, true); !loaded {
							enqueue(assetTask{srcPath: a.Path, relPath: rel, info: a.Info})
						}
					}
				}
			case <-dCtx.Done():
				return dCtx.Err()
			}
			return nil
		})
	}

	err = discoveryGroup.Wait()
	walkerWg.Wait()
	close(assetChan)
	workerWg.Wait()

	if debugAssets {
		s.logger.Info("Static discovery stats",
			"site_dir", siteStaticDir,
			"theme_dir", themeDir,
			"site_files", atomic.LoadInt64(&siteFiles),
			"theme_files", atomic.LoadInt64(&themeFiles),
			"site_enqueued", atomic.LoadInt64(&siteEnqueued),
			"theme_enqueued", atomic.LoadInt64(&themeEnqueued),
			"rel_errors", atomic.LoadInt64(&relErrs),
			"site_samples", siteSamples,
			"theme_samples", themeSamples,
		)
	}

	if err != nil {
		return nil, err
	}

	s.copyCriticalAssets()

	if err := copyGroup.Wait(); err != nil {
		return nil, err
	}

	return imageQueue, nil
}

// Build executes the asset processing pipeline with two-phase image processing:
//
// Phase 1 (discovery): Copy non-image assets and cached images immediately.
// Closes discoveryReady so post-processing can start while images are still processing.
//
// Phase 2 (background): Process cache-miss images (decode + resize + WebP encode).
// Runs concurrently with post rendering. Closes assetsReady when complete.
func (s *assetService) Build(ctx context.Context) error {
	return s.BuildWithOptions(ctx, false)
}

// BuildWithOptions executes the asset processing pipeline with optional image processing.
// When skipImages is true, image processing is skipped but esbuild still runs to produce
// hashed assets for CSS/JS.
func (s *assetService) BuildWithOptions(ctx context.Context, skipImages bool) error {
	g, gCtx := errgroup.WithContext(ctx)

	if s.discoveryReady == nil {
		s.discoveryReady = make(chan struct{})
	}

	g.Go(func() error {
		copyTimer := timeutil.StartPhase("Asset discovery and copy")
		defer copyTimer.Stop()

		defer func() {
			if s.discoveryReady != nil {
				close(s.discoveryReady)
				s.discoveryReady = nil
			}
		}()

		imageQueue, err := s.syncStaticAssets(ctx, gCtx, skipImages)
		if err != nil {
			return err
		}

		// Close discoveryReady so post-processing can start while images are still processing
		if s.discoveryReady != nil {
			close(s.discoveryReady)
			s.discoveryReady = nil
		}

		if !skipImages && len(imageQueue) > 0 {
			numWorkers := s.cfg.ImageWorkers
			if numWorkers <= 0 {
				numWorkers = runtime.NumCPU()
			}
			imgGroup, imgCtx := errgroup.WithContext(gCtx)
			imgGroup.SetLimit(max(numWorkers, 1))
			imgChan := make(chan imageCopyTask, len(imageQueue))
			for _, t := range imageQueue {
				imgChan <- t
			}
			close(imgChan)
			for range max(numWorkers, 1) {
				imgGroup.Go(func() error {
					for task := range imgChan {
						if err := assets.ProcessCacheMissImage(task.opts); err != nil && imgCtx.Err() == nil {
							return err
						}
					}
					return nil
				})
			}
			if err := imgGroup.Wait(); err != nil {
				return err
			}
		}

		return nil
	})

	g.Go(func() error {
		esbuildTimer := timeutil.StartPhase("Asset esbuild")
		defer esbuildTimer.Stop()
		_, err := s.buildEsbuildAssets(false)
		return err
	})

	err := g.Wait()

	if s.assetsReady != nil {
		close(s.assetsReady)
		s.assetsReady = nil
	}

	return err
}

func (s *assetService) copyCriticalAssets() {
	if s.cfg.Logo != "" {
		if err := s.copyFileOrLink(s.cfg.Logo, s.cfg.Logo); err != nil {
			s.logger.Warn("Failed to copy logo", "src", s.cfg.Logo, "error", err)
		}
	}
	faviconPath := filepath.Join(s.cfg.StaticDir, "images/favicon.png")
	if exists, _ := afero.Exists(s.sourceFs, faviconPath); exists {
		if err := s.copyFileOrLink(faviconPath, "static/images/favicon.png"); err != nil {
			s.logger.Warn("Failed to copy favicon", "src", faviconPath, "error", err)
		}
	}
}

func (s *assetService) copyFileOrLink(src, dst string) error {
	info, err := s.sourceFs.Stat(src)
	if err != nil {
		return err
	}
	err = fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   s.sourceFs,
		Sink:    s.sink,
		SrcPath: src,
		DstPath: dst,
		ModTime: info.ModTime().UnixNano(),
		OnWrite: s.renderer.RegisterFile,
	})
	if err == nil && s.metrics != nil {
		s.metrics.IncrementAssetsProcessed()
	}
	return err
}

func (s *assetService) buildEsbuildAssets(force bool) (map[string]string, error) {
	destStaticDir, _ := filepath.Abs(filepath.Join(s.cfg.OutputDir, "static"))

	srcDir := s.cfg.StaticDir
	if srcDir == "" {
		srcDir = "themes/blog/static"
	}

	var sched scheduler.BuildScheduler
	if s.ctx != nil {
		sched = s.ctx.Scheduler
	}

	var onAssetProcessed func()
	if s.metrics != nil {
		onAssetProcessed = s.metrics.IncrementAssetsProcessed
	}

	assets, assetErr := fspkg.BuildAssetsEsbuild(s.sourceFs, s.sink, srcDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force, sched, onAssetProcessed)
	if assetErr == nil {
		s.renderer.SetAssets(assets)
	}
	return assets, assetErr
}

func (s *assetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	syncTimer := timeutil.StartPhase("Asset sync")
	defer syncTimer.Stop()

	imageQueue, err := s.syncStaticAssets(ctx, ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to sync static assets: %w", err)
	}

	for _, task := range imageQueue {
		if err := assets.ProcessCacheMissImage(task.opts); err != nil {
			return nil, fmt.Errorf("failed to process cache-miss image: %w", err)
		}
	}

	esbuildTimer := timeutil.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return s.buildEsbuildAssets(true)
}

func (s *assetService) BuildForAssetChangeWithOptions(ctx context.Context, forceImages bool) (map[string]string, error) {
	syncTimer := timeutil.StartPhase("Asset sync")
	defer syncTimer.Stop()

	imageQueue, err := s.syncStaticAssets(ctx, ctx, !forceImages)
	if err != nil {
		return nil, fmt.Errorf("failed to sync static assets: %w", err)
	}

	if forceImages || len(imageQueue) > 0 {
		for _, task := range imageQueue {
			if err := assets.ProcessCacheMissImage(task.opts); err != nil {
				return nil, fmt.Errorf("failed to process cache-miss image: %w", err)
			}
		}
	}

	esbuildTimer := timeutil.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return s.buildEsbuildAssets(true)
}

func ComputeStaticFingerprint(sourceFs afero.Fs, dirs []string) (string, error) {
	hasher := xxh3.New()
	var fileCount int

	for _, dir := range dirs {
		dir = fspkg.NormalizePath(dir)
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return nil
			}
			relPath = filepath.ToSlash(relPath)
			if _, err := fmt.Fprintf(hasher, "%s:%d:%d;", relPath, info.Size(), info.ModTime().UnixNano()); err != nil {
				return err
			}
			fileCount++
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to walk directory %s: %w", dir, err)
		}
	}

	hash := hasher.Sum128()
	b := hash.Bytes()
	return hex.EncodeToString(b[:]), nil
}

func GetStaticDirs(cfg *config.Config) []string {
	var dirs []string

	themeStatic := filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	if cfg.StaticDir != "" {
		themeStatic = cfg.StaticDir
	}
	if _, err := os.Stat(themeStatic); err == nil {
		dirs = append(dirs, themeStatic)
	}

	siteStatic := "static"
	if cfg.SiteRoot != "" {
		siteStatic = filepath.Join(cfg.SiteRoot, "static")
	}
	if _, err := os.Stat(siteStatic); err == nil {
		dirs = append(dirs, siteStatic)
	}

	return dirs
}

func LoadStaticFingerprint(cacheDir string) (string, error) {
	if cacheDir == "" {
		return "", fmt.Errorf("cache directory not set")
	}
	fingerprintPath := filepath.Join(cacheDir, "static-fingerprint")
	data, err := os.ReadFile(fingerprintPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func SaveStaticFingerprint(cacheDir, fingerprint string) error {
	if cacheDir == "" {
		return fmt.Errorf("cache directory not set")
	}
	fingerprintPath := filepath.Join(cacheDir, "static-fingerprint")
	return os.WriteFile(fingerprintPath, []byte(fingerprint), 0644)
}
