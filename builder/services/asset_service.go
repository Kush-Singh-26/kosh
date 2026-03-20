package services

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// AssetServiceOption configures optional parameters for AssetService
//
// Note: Channels (AssetsReady, ContentAssetsChan) should still use
// WithAssetsReadySignal and WithContentAssetsChannel options.
type AssetServiceOption func(*assetService)

// WithMetrics sets the build metrics collector
func WithMetrics(m *metrics.BuildMetrics) AssetServiceOption {
	return func(s *assetService) { s.metrics = m }
}

// WithAssetsReadySignal sets the channel signaled when assets are ready
//
// Note: This option is still required for channel-based coordination.
func WithAssetsReadySignal(ch chan struct{}) AssetServiceOption {
	return func(s *assetService) { s.assetsReady = ch }
}

// WithContentAssetsChannel sets the channel for content asset notifications
//
// Note: This option is still required for channel-based coordination.
func WithContentAssetsChannel(ch <-chan []models.ScannedAsset) AssetServiceOption {
	return func(s *assetService) { s.contentAssetsChan = ch }
}

// assetService implements AssetService
type assetService struct {
	ctx               *buildCtx.BuildContext
	sourceFs          afero.Fs
	sink              fspkg.ArtifactSink
	cfg               *config.Config
	renderer          RenderService
	logger            *slog.Logger
	metrics           *metrics.BuildMetrics
	contentAssetsChan <-chan []models.ScannedAsset
	contentAssets     []models.ScannedAsset
	// assetsReady is owned by AssetService, created per-build and closed when assets are ready.
	// RenderService and PostService wait on this channel but do not own its lifecycle.
	assetsReady chan struct{}
}

// NewAssetService creates a new AssetService with the given dependencies.
//
// Channel Ownership:
//   - AssetsReady: must be set via WithAssetsReadySignal option
//   - ContentAssetsChan: must be set via WithContentAssetsChannel option
func NewAssetService(deps AssetServiceDependencies, opts ...AssetServiceOption) AssetService {
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
func (s *assetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	s.contentAssetsChan = ch
}

func (s *assetService) Build(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// 1. Unified Asset Copy Phase
	g.Go(func() error {
		copyTimer := timeutil.StartPhase("Asset copy unified")
		defer copyTimer.Stop()

		themeDir := s.cfg.StaticDir
		if themeDir == "" {
			themeDir = "themes/blog/static"
		}

		// Calculate worker count early for discovery phase
		numWorkers := s.cfg.ImageWorkers
		if numWorkers <= 0 {
			numWorkers = runtime.NumCPU()
		}

		type assetTask struct {
			srcPath string
			info    fs.FileInfo
		}
		// Use a sync.Map for thread-safe discovery to avoid any potential race conditions
		var assetSyncMap sync.Map

		// Discovery with higher concurrency for modern SSDs
		discoveryGroup, dCtx := errgroup.WithContext(gCtx)
		walkConcurrency := max(numWorkers/2, 4)

		discoveryGroup.Go(func() error {
			exists, _ := afero.Exists(s.sourceFs, themeDir)
			if exists {
				if err := fspkg.ParallelWalk(dCtx, s.sourceFs, themeDir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					if filepath.Base(path) == "search.wasm" {
						return nil
					}
					rel, _ := fspkg.SafeRel(themeDir, path)
					assetSyncMap.Store("static/"+rel, assetTask{srcPath: path, info: info})
					return nil
				}); err != nil {
					s.logger.Log(dCtx, slog.LevelWarn, "theme asset walk error", "dir", themeDir, "err", err)
				}
			}
			return nil
		})

		if themeDir != "static" {
			discoveryGroup.Go(func() error {
				exists, _ := afero.Exists(s.sourceFs, "static")
				if exists {
					if err := fspkg.ParallelWalk(dCtx, s.sourceFs, "static", walkConcurrency, func(path string, info fs.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						if filepath.Base(path) == "search.wasm" {
							return nil
						}
						rel, _ := fspkg.SafeRel("static", path)
						assetSyncMap.Store("static/"+rel, assetTask{srcPath: path, info: info})
						return nil
					}); err != nil {
						s.logger.Log(dCtx, slog.LevelWarn, "static asset walk error", "dir", "static", "err", err)
					}
				}
				return nil
			})
		}

		_ = discoveryGroup.Wait()

		// Convert sync.Map to a regular map for the copy phase
		assetMap := make(map[string]assetTask, 256)
		assetSyncMap.Range(func(key, value any) bool {
			assetMap[key.(string)] = value.(assetTask)
			return true
		})

		// Wait for content assets if channel provided
		if s.contentAssetsChan != nil {
			select {
			case assets, ok := <-s.contentAssetsChan:
				if ok && assets != nil {
					s.contentAssets = assets
				} else {
					s.contentAssets = []models.ScannedAsset{}
				}
			case <-gCtx.Done():
				return gCtx.Err()
			}
		}

		for _, a := range s.contentAssets {
			rel, _ := fspkg.SafeRel(s.cfg.ContentDir, a.Path)
			assetMap[rel] = assetTask{srcPath: a.Path, info: a.Info}
		}

		// Dispatch with STRICT limit (128 for Windows to avoid I/O contention)
		copyGroup, copyCtx := errgroup.WithContext(gCtx)
		copyGroup.SetLimit(128)

		for rel, task := range assetMap {
			r, t := rel, task
			copyGroup.Go(func() error {
				dst := filepath.Join(s.cfg.OutputDir, r)
				opts := fspkg.CopyOptions{
					Compress:     s.cfg.CompressImages,
					CacheDir:     s.cfg.CacheDir + "/images",
					WebPQuality:  s.cfg.WebPQuality,
					Metrics:      s.metrics,
					OnWrite:      s.renderer.RegisterFile,
					ImageWorkers: s.cfg.ImageWorkers,
				}
				return fspkg.CopyFileWithOptionalImageProcessing(fspkg.ProcessImageOptions{
					Ctx:     copyCtx,
					SrcFs:   s.sourceFs,
					Sink:    s.sink,
					SrcPath: t.srcPath,
					DstPath: dst,
					SrcInfo: t.info,
					Opts:    opts,
					Scheduler: func() scheduler.BuildScheduler {
						if s.ctx != nil {
							return s.ctx.Scheduler
						}
						return scheduler.GetGlobalScheduler()
					}(),
				})
			})
		}

		s.copyCriticalAssets()
		return copyGroup.Wait()
	})

	// 2. Esbuild
	g.Go(func() error {
		esbuildTimer := timeutil.StartPhase("Asset esbuild")
		defer esbuildTimer.Stop()
		_, err := s.buildEsbuildAssets(false)
		if s.assetsReady != nil {
			close(s.assetsReady)
			s.assetsReady = nil
		}
		return err
	})

	return g.Wait()
}

func (s *assetService) copyCriticalAssets() {
	if s.cfg.Logo != "" {
		_ = s.copyFileOrLink(s.cfg.Logo, s.cfg.Logo)
	}
	faviconPath := filepath.Join(s.cfg.StaticDir, "images/favicon.png")
	if exists, _ := afero.Exists(s.sourceFs, faviconPath); exists {
		_ = s.copyFileOrLink(faviconPath, "static/images/favicon.png")
	}
}

func (s *assetService) copyFileOrLink(src, dst string) error {
	info, err := s.sourceFs.Stat(src)
	if err != nil {
		return err
	}
	return fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   s.sourceFs,
		Sink:    s.sink,
		SrcPath: src,
		DstPath: dst,
		ModTime: info.ModTime().UnixNano(),
		OnWrite: s.renderer.RegisterFile,
	})
}

func (s *assetService) buildEsbuildAssets(force bool) (map[string]string, error) {
	destStaticDir, _ := filepath.Abs(filepath.Join(s.cfg.OutputDir, "static"))
	assets, assetErr := fspkg.BuildAssetsEsbuild(s.sourceFs, s.sink, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force)
	if assetErr == nil {
		s.renderer.SetAssets(assets)
	}
	return assets, assetErr
}

func (s *assetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	esbuildTimer := timeutil.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return s.buildEsbuildAssets(true)
}
