package asset

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

func (s *assetService) Build(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// discoveryReady signals that the image/WebP rewrite map is populated.
	// This allows post-processing to begin before all images are fully compressed.
	s.discoveryReady = make(chan struct{})

	// 1. Unified Asset Discovery and Copy Phase (Pipelined)
	g.Go(func() error {
		copyTimer := timeutil.StartPhase("Asset discovery and copy")
		defer copyTimer.Stop()

		themeDir := s.cfg.StaticDir
		if themeDir == "" {
			themeDir = "themes/blog/static"
		}

		numWorkers := s.cfg.ImageWorkers
		if numWorkers <= 0 {
			numWorkers = runtime.NumCPU()
		}

		// Channel for streaming discovery to copy workers
		assetChan := make(chan assetTask, 128)
		// Track seen files to handle overrides (project static > theme static)
		var seen sync.Map

		copyGroup, copyCtx := errgroup.WithContext(gCtx)
		// Dispatch with STRICT limit (128 for Windows to avoid I/O contention)
		copyGroup.SetLimit(128)

		// Start copy workers before discovery begins
		discoveryWg := sync.WaitGroup{}
		discoveryWg.Add(1)
		go func() {
			defer discoveryWg.Done()
			for task := range assetChan {
				t := task
				copyGroup.Go(func() error {
					dst := filepath.Join(s.cfg.OutputDir, t.relPath)
					opts := assets.CopyOptions{
						Compress:     s.cfg.CompressImages,
						MinifySVGs:   s.cfg.MinifySVGs,
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

		// Discovery phase: walk project static FIRST (overrides), then theme static
		discoveryGroup, dCtx := errgroup.WithContext(gCtx)
		walkConcurrency := max(numWorkers/2, 4)

		// Project static discovery (High Priority)
		if themeDir != "static" {
			discoveryGroup.Go(func() error {
				return s.walkDirStreaming(dCtx, "static", "static", walkConcurrency, assetChan, &seen)
			})
		}

		// Theme static discovery
		discoveryGroup.Go(func() error {
			return s.walkDirStreaming(dCtx, themeDir, themeDir, walkConcurrency, assetChan, &seen)
		})

		// Wait for content assets (passed from Scanner)
		if s.contentAssetsChan != nil {
			discoveryGroup.Go(func() error {
				select {
				case assets, ok := <-s.contentAssetsChan:
					if ok && assets != nil {
						for _, a := range assets {
							rel, _ := fspkg.SafeRel(s.cfg.ContentDir, a.Path)
							if _, loaded := seen.LoadOrStore(rel, true); !loaded {
								assetChan <- assetTask{srcPath: a.Path, relPath: rel, info: a.Info}
							}
						}
					}
				case <-dCtx.Done():
					return dCtx.Err()
				}
				return nil
			})
		}

		err := discoveryGroup.Wait()
		close(assetChan)
		discoveryWg.Wait()

		// Signal discovery complete so post-processing can start
		// while image compression continues in the background
		close(s.discoveryReady)
		s.discoveryReady = nil // consumed, prevent reuse

		if err != nil {
			return err
		}

		s.copyCriticalAssets()
		return copyGroup.Wait()
	})

	// 2. Esbuild Bundling (CSS/JS)
	g.Go(func() error {
		esbuildTimer := timeutil.StartPhase("Asset esbuild")
		defer esbuildTimer.Stop()
		_, err := s.buildEsbuildAssets(false)
		return err
	})

	err := g.Wait()

	// Close assetsReady signal only after ALL processing (including esbuild) is complete.
	// This ensures RenderService waits for CSS/JS hashes before committing.
	if s.assetsReady != nil {
		close(s.assetsReady)
		s.assetsReady = nil
	}

	return err
}

func (s *assetService) walkDirStreaming(ctx context.Context, dir, prefix string, concurrency int, assetChan chan<- assetTask, seen *sync.Map) error {
	exists, _ := afero.Exists(s.sourceFs, dir)
	if !exists {
		return nil
	}

	return fspkg.ParallelWalk(ctx, s.sourceFs, dir, concurrency, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "search.wasm" {
			return nil
		}
		rel, _ := fspkg.SafeRel(dir, path)
		fullRel := "static/" + rel
		if _, loaded := seen.LoadOrStore(fullRel, true); !loaded {
			select {
			case assetChan <- assetTask{srcPath: path, relPath: fullRel, info: info}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
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

	srcDir := s.cfg.StaticDir
	if srcDir == "" {
		srcDir = "themes/blog/static"
	}

	var sched scheduler.BuildScheduler
	if s.ctx != nil {
		sched = s.ctx.Scheduler
	}

	assets, assetErr := fspkg.BuildAssetsEsbuild(s.sourceFs, s.sink, srcDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force, sched)
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
