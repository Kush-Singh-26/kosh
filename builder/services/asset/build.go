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
func (service *assetService) Build(ctx context.Context) error {
	return service.BuildWithOptions(ctx, false)
}

// BuildWithOptions executes the asset processing pipeline with optional image processing.
// When skipImages is true, image processing is skipped but esbuild still runs to produce
// hashed assets for CSS/JS.
func (service *assetService) BuildWithOptions(ctx context.Context, skipImages bool) error {
	errorGroup, groupCtx := errgroup.WithContext(ctx)

	if service.discoveryReady == nil {
		service.discoveryReady = make(chan struct{})
	}

	errorGroup.Go(func() error {
		copyTimer := timeutil.StartPhase("Asset discovery and copy")
		defer copyTimer.Stop()

		// Load asset manifest
		if service.cfg.CacheDir != "" {
			manifest, err := assets.LoadManifest(service.cfg.CacheDir + "/assets")
			if err != nil {
				service.logger.Warn("Failed to load asset manifest", "error", err)
			} else {
				service.manifest = manifest
			}
		}

		defer func() {
			if service.discoveryReady != nil {
				close(service.discoveryReady)
				service.discoveryReady = nil
			}
			// Save asset manifest
			if service.manifest != nil && service.cfg.CacheDir != "" {
				if err := assets.SaveManifest(service.cfg.CacheDir+"/assets", service.manifest); err != nil {
					service.logger.Warn("Failed to save asset manifest", "error", err)
				}
			}
		}()

		var imageChannel chan imageCopyTask
		var imgGroup *errgroup.Group
		var imgGroupCtx context.Context

		if !skipImages {
			numWorkers := service.cfg.ImageWorkers
			if numWorkers <= 0 {
				numWorkers = runtime.NumCPU()
			}
			imgGroup, imgGroupCtx = errgroup.WithContext(groupCtx)
			imgGroup.SetLimit(max(numWorkers, 1))
			imageChannel = make(chan imageCopyTask, 1024) // Buffer to allow walk to proceed

			processedCount := atomic.Int32{}

			for range max(numWorkers, 1) {
				imgGroup.Go(func() error {
					for task := range imageChannel {
						if err := assets.ProcessCacheMissImage(task.opts); err != nil && imgGroupCtx.Err() == nil {
							return err
						}
						if service.reporter != nil {
							// Note: totalCount is unknown during discovery, using -1 or approximate
							currentCount := int(processedCount.Add(1))
							service.reporter.UpdateProgress(ui.PhaseAssets, currentCount, 0, task.opts.SrcPath)
						}
					}
					return nil
				})
			}
		}

		err := service.syncStaticAssets(ctx, groupCtx, skipImages, imageChannel)
		if imageChannel != nil {
			close(imageChannel)
		}
		if err != nil {
			return err
		}

		// Close discoveryReady so post-processing can start while images are still processing.
		if service.discoveryReady != nil {
			close(service.discoveryReady)
			service.discoveryReady = nil
		}

		if imgGroup != nil {
			if err := imgGroup.Wait(); err != nil {
				return err
			}
		}

		return nil
	})

	errorGroup.Go(func() error {
		esbuildTimer := timeutil.StartPhase("Asset esbuild")
		defer esbuildTimer.Stop()
		_, err := service.buildEsbuildAssets(false)
		return err
	})

	err := errorGroup.Wait()

	if service.assetsReady != nil {
		close(service.assetsReady)
		service.assetsReady = nil
	}

	return err
}

// BuildForAssetChange rebuilds assets when a static asset change is detected.
func (service *assetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	syncTimer := timeutil.StartPhase("Asset sync")
	defer syncTimer.Stop()

	imageChannel := make(chan imageCopyTask, 64)
	imgGroup, imgGroupCtx := errgroup.WithContext(ctx)
	imgGroup.SetLimit(max(service.cfg.ImageWorkers, 1))

	imgGroup.Go(func() error {
		for task := range imageChannel {
			if err := assets.ProcessCacheMissImage(task.opts); err != nil && imgGroupCtx.Err() == nil {
				return err
			}
		}
		return nil
	})

	err := service.syncStaticAssets(ctx, ctx, false, imageChannel)
	close(imageChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to sync static assets: %w", err)
	}

	if err := imgGroup.Wait(); err != nil {
		return nil, fmt.Errorf("failed to process images: %w", err)
	}

	esbuildTimer := timeutil.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return service.buildEsbuildAssets(true)
}

// BuildForAssetChangeWithOptions rebuilds assets after changes, optionally forcing images.
func (service *assetService) BuildForAssetChangeWithOptions(ctx context.Context, forceImages bool) (map[string]string, error) {
	syncTimer := timeutil.StartPhase("Asset sync")
	defer syncTimer.Stop()

	imageChannel := make(chan imageCopyTask, 64)
	imgGroup, imgGroupCtx := errgroup.WithContext(ctx)
	imgGroup.SetLimit(max(service.cfg.ImageWorkers, 1))

	imgGroup.Go(func() error {
		for task := range imageChannel {
			if err := assets.ProcessCacheMissImage(task.opts); err != nil && imgGroupCtx.Err() == nil {
				return err
			}
		}
		return nil
	})

	err := service.syncStaticAssets(ctx, ctx, !forceImages, imageChannel)
	close(imageChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to sync static assets: %w", err)
	}

	if err := imgGroup.Wait(); err != nil {
		return nil, fmt.Errorf("failed to process images: %w", err)
	}

	esbuildTimer := timeutil.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return service.buildEsbuildAssets(true)
}

func (service *assetService) buildEsbuildAssets(forceBuild bool) (map[string]string, error) {
	destinationStaticDir, _ := filepath.Abs(filepath.Join(service.cfg.OutputDir, "static"))

	sourceDir := service.cfg.StaticDir
	if sourceDir == "" {
		sourceDir = "themes/blog/static"
	}

	var buildScheduler scheduler.BuildScheduler
	if service.ctx != nil {
		buildScheduler = service.ctx.Scheduler
	}

	var onAssetProcessed func()
	if service.metrics != nil {
		onAssetProcessed = service.metrics.IncrementAssetsProcessed
	}

	assetsMap, err := fspkg.BuildAssetsEsbuild(fspkg.BuildAssetsOptions{
		SrcFs:            service.sourceFs,
		Sink:             service.sink,
		SrcDir:           sourceDir,
		DestDir:          destinationStaticDir,
		Minify:           service.cfg.ShouldCompressImages,
		OnWrite:          service.renderer.RegisterFile,
		CacheDir:         service.cfg.CacheDir + "/assets",
		Force:            forceBuild,
		Sched:            buildScheduler,
		OnAssetProcessed: onAssetProcessed,
	})
	if err == nil {
		service.renderer.SetAssets(assetsMap)
	}
	return assetsMap, err
}

func (service *assetService) copyFileOrLink(sourcePath, destinationPath string) error {
	info, err := service.sourceFs.Stat(sourcePath)
	if err != nil {
		return err
	}
	err = fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   service.sourceFs,
		Sink:    service.sink,
		SrcPath: sourcePath,
		DstPath: destinationPath,
		ModTime: info.ModTime().UnixNano(),
		OnWrite: service.renderer.RegisterFile,
	})
	if err == nil && service.metrics != nil {
		service.metrics.IncrementAssetsProcessed()
	}
	return err
}
