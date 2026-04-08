package asset

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

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

		// Close discoveryReady so post-processing can start while images are still processing.
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

			processed := atomic.Int32{}
			total := len(imageQueue)

			for range max(numWorkers, 1) {
				imgGroup.Go(func() error {
					for task := range imgChan {
						if err := assets.ProcessCacheMissImage(task.opts); err != nil && imgCtx.Err() == nil {
							return err
						}
						if s.reporter != nil {
							curr := int(processed.Add(1))
							s.reporter.UpdateProgress(ui.PhaseAssets, curr, total, task.opts.SrcPath)
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

// BuildForAssetChange rebuilds assets when a static asset change is detected.
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

// BuildForAssetChangeWithOptions rebuilds assets after changes, optionally forcing images.
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

	assets, assetErr := fspkg.BuildAssetsEsbuild(fspkg.BuildAssetsOptions{
		SrcFs:            s.sourceFs,
		Sink:             s.sink,
		SrcDir:           srcDir,
		DestDir:          destStaticDir,
		Minify:           s.cfg.CompressImages,
		OnWrite:          s.renderer.RegisterFile,
		CacheDir:         s.cfg.CacheDir + "/assets",
		Force:            force,
		Sched:            sched,
		OnAssetProcessed: onAssetProcessed,
	})
	if assetErr == nil {
		s.renderer.SetAssets(assets)
	}
	return assets, assetErr
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
