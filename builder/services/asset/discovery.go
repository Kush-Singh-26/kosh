package asset

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/async"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

const (
	assetChanBuffer         = 512
	assetCopyGroupLimit     = 256
	assetWalkConcurrencyDiv = 2
	assetMinWalkConcurrency = 4
	defaultWalkConcurrency  = 1
	discoverySampleLimit    = 5
)

type assetTask struct {
	sourcePath   string
	relativePath string
	info         fs.FileInfo
}

type imageCopyTask struct {
	task assetTask
	opts assets.ProcessImageOptions
}

type syncContext struct {
	seen sync.Map

	siteFiles     int64
	themeFiles    int64
	siteEnqueued  int64
	themeEnqueued int64
	relErrs       int64

	siteSamples  []string
	themeSamples []string
	sampleMu     sync.Mutex // protects siteSamples and themeSamples

	imageTasks  []imageCopyTask
	imageTaskMu sync.Mutex
}

func (service *assetService) isWebPCandidate(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".jpg" && extension != ".jpeg" && extension != ".png" {
		return false
	}

	// Exclude critical assets from WebP conversion to preserve compatibility
	// with non-img tags (icons, social sharing, etc.)
	base := strings.ToLower(filepath.Base(path))
	if base == "icon-192.png" || base == "icon-512.png" {
		return false
	}

	if service.cfg != nil && service.cfg.Logo != "" {
		logoBase := strings.ToLower(filepath.Base(service.cfg.Logo))
		if base == logoBase {
			return false
		}
	}

	return true
}

// syncStaticAssets discovers and copies all static assets to the sink.
// Concurrent image processing is enabled by providing a non-nil imageChan.
func (service *assetService) syncStaticAssets(ctx context.Context, backgroundCtx context.Context, skipImages bool, imageChan chan<- imageCopyTask) error {
	dirs := service.getStaticSourceDirs()
	debugAssets := service.cfg.Debug

	syncContextInstance := &syncContext{}

	workerCount := service.cfg.ImageWorkers
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}

	assetChan := make(chan assetTask, assetChanBuffer)
	copyGroup, copyContext := errgroup.WithContext(ctx)
	copyGroup.SetLimit(assetCopyGroupLimit)

	workerWg := service.setupAssetWorker(assetChan, copyGroup, copyContext)

	walkerWg := sync.WaitGroup{}
	discoveryGroup, discoveryContext := errgroup.WithContext(ctx)
	walkConcurrency := max(workerCount/assetWalkConcurrencyDiv, assetMinWalkConcurrency)

	enqueue := service.setupImageEnqueue(backgroundCtx, skipImages, syncContextInstance, assetChan, imageChan)
	walkFunc := service.setupDiscoveryWalk(discoveryWalkOptions{
		walkerWg:        &walkerWg,
		walkConcurrency: walkConcurrency,
		debugAssets:     debugAssets,
		syncCtx:         syncContextInstance,
		enqueue:         enqueue,
	})

	for _, dir := range dirs {
		d := dir
		discoveryGroup.Go(func() error { return walkFunc(ctx, d, true) })
	}

	if service.contentAssetsChan != nil {
		discoveryGroup.Go(func() error {
			return service.discoverContentAssets(discoveryContext, syncContextInstance, enqueue)
		})
	}

	err := discoveryGroup.Wait()
	walkerWg.Wait()

	// 5. Image Priority Queue (Fat tail optimization)
	// Larger images are enqueued first to ensure they start processing as early as possible.
	if imageChan != nil {
		syncContextInstance.imageTaskMu.Lock()
		tasks := syncContextInstance.imageTasks
		syncContextInstance.imageTasks = nil // Free memory
		syncContextInstance.imageTaskMu.Unlock()

		slices.SortFunc(tasks, func(a, b imageCopyTask) int {
			if a.task.info.Size() > b.task.info.Size() {
				return -1
			}
			if a.task.info.Size() < b.task.info.Size() {
				return 1
			}
			return 0
		})

		for _, t := range tasks {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case imageChan <- t:
			}
		}
	}

	close(assetChan)
	workerWg.Wait()

	if debugAssets {
		service.logDiscoveryStats(dirs, syncContextInstance)
	}

	if err != nil {
		return err
	}

	service.copyCriticalAssets()

	return copyGroup.Wait()
}

func (service *assetService) getStaticSourceDirs() []string {
	var dirs []string

	themeDir := service.cfg.StaticDir
	if themeDir == "" {
		themeDir = "themes/blog/static"
	}
	if _, err := os.Stat(themeDir); err == nil {
		dirs = append(dirs, themeDir)
	}

	// 1. Check SiteRoot (current working directory)
	siteStaticDir := "static"
	if service.cfg.SiteRoot != "" {
		siteStaticDir = filepath.Join(service.cfg.SiteRoot, "static")
	}
	if _, err := os.Stat(siteStaticDir); err == nil {
		dirs = append(dirs, siteStaticDir)
	}

	assetsDir := "assets"
	if service.cfg.SiteRoot != "" {
		assetsDir = filepath.Join(service.cfg.SiteRoot, "assets")
	}
	if _, err := os.Stat(assetsDir); err == nil {
		dirs = append(dirs, assetsDir)
	}

	// 2. Fallback: Check parent of ContentDir (handles blogs-src/static style layouts)
	if service.cfg.ContentDir != "" {
		contentParent := filepath.Dir(service.cfg.ContentDir)
		if contentParent != service.cfg.SiteRoot {
			fallbackStatic := filepath.Join(contentParent, "static")
			if _, err := os.Stat(fallbackStatic); err == nil {
				dirs = append(dirs, fallbackStatic)
			}
			fallbackAssets := filepath.Join(contentParent, "assets")
			if _, err := os.Stat(fallbackAssets); err == nil {
				dirs = append(dirs, fallbackAssets)
			}
		}
	}

	if service.cfg.Server.RootDirectory != "" {
		parentAssets := filepath.Join(service.cfg.Server.RootDirectory, "assets")
		if _, err := os.Stat(parentAssets); err == nil {
			dirs = append(dirs, parentAssets)
		}
	}

	return dirs
}

func (service *assetService) setupAssetWorker(assetChan <-chan assetTask, group *errgroup.Group, ctx context.Context) *sync.WaitGroup {
	waitGroupInstance := &sync.WaitGroup{}
	waitGroupInstance.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    service.logger,
		Operation: "asset worker",
		Fn: func() error {
			for task := range assetChan {
				currentTask := task
				group.Go(func() error {
					destinationPath := filepath.Join(service.cfg.OutputDir, currentTask.relativePath)
					options := assets.CopyOptions{
						Compress:     service.cfg.ShouldCompressImages,
						MinifySVGs:   service.cfg.ShouldMinifySVGs,
						KeepOriginal: false,
						CacheDir:     service.cfg.CacheDir + "/images",
						WebPQuality:  service.cfg.WebPQuality,
						Metrics:      service.metrics,
						OnWrite:      service.renderer.RegisterFile,
						ImageWorkers: service.cfg.ImageWorkers,
						Manifest:     service.manifest,
					}
					if err := assets.CopyFileWithOptionalImageProcessing(assets.ProcessImageOptions{
						Ctx:     ctx,
						SrcFs:   service.sourceFs,
						Sink:    service.sink,
						SrcPath: currentTask.sourcePath,
						DstPath: destinationPath,
						RelPath: currentTask.relativePath,
						SrcInfo: currentTask.info,
						Opts:    options,
						Scheduler: func() scheduler.BuildScheduler {
							if service.ctx != nil {
								return service.ctx.Scheduler
							}
							return nil
						}(),
					}); err != nil {
						return err
					}

					// Update manifest for non-image assets (images are handled by image cache)
					if service.manifest != nil && !service.isWebPCandidate(currentTask.sourcePath) {
						service.manifest.Set(currentTask.sourcePath, assets.AssetManifestEntry{
							Size:    currentTask.info.Size(),
							ModTime: currentTask.info.ModTime().UnixNano(),
						})
					}
					return nil
				})
			}
			return nil
		},
		Cleanup: waitGroupInstance.Done,
	})
	return waitGroupInstance
}

func (service *assetService) setupImageEnqueue(backgroundCtx context.Context, skipImages bool, syncContextInstance *syncContext, assetChan chan<- assetTask, imageChan chan<- imageCopyTask) func(assetTask) {
	return func(task assetTask) {
		if service.cfg.ShouldCompressImages && service.isWebPCandidate(task.sourcePath) {
			relativeSource := "/" + strings.TrimPrefix(filepath.ToSlash(task.relativePath), "/")
			relativeDestination := relativeSource[:len(relativeSource)-len(filepath.Ext(relativeSource))] + ".webp"
			assets.RecordConvertedImage(relativeSource, relativeDestination)
			assets.RecordConvertedImage(strings.TrimPrefix(relativeSource, "/"), relativeDestination)

			outerDestination := filepath.Join(service.cfg.OutputDir, task.relativePath)
			destinationWebPPath := outerDestination[:len(outerDestination)-len(filepath.Ext(outerDestination))] + ".webp"

			if skipImages && service.cfg.IsDev {
				if _, err := service.sink.Stat(destinationWebPPath); err == nil {
					service.sink.Register(destinationWebPPath)
					if service.renderer != nil {
						service.renderer.RegisterFile(destinationWebPPath)
					}
					return
				}
			}

			err := assets.CopyFromDiskCache(assets.CopyFromDiskCacheOptions{
				SrcFs:        service.sourceFs,
				Sink:         service.sink,
				RelPath:      task.relativePath,
				SrcPath:      task.sourcePath,
				DstPath:      destinationWebPPath,
				CacheDir:     service.cfg.CacheDir + "/images",
				SrcInfo:      task.info,
				Metrics:      service.metrics,
				OnWrite:      service.renderer.RegisterFile,
				KeepOriginal: service.cfg.IsDev || service.cfg.Features.UseRawMarkdown,
				MuteMetrics:  skipImages,
			})
			if err == nil {
				return
			}
			if !errors.Is(err, assets.ErrCacheMiss) {
				if _, loaded := service.warnOnce.LoadOrStore("cache-fail:"+task.sourcePath, true); !loaded {
					service.logger.Warn("Disk cache lookup failed", "path", task.sourcePath, "error", err)
				}
			}

			if imageChan != nil {
				imageOptions := assets.ProcessImageOptions{
					Ctx:     backgroundCtx,
					SrcFs:   service.sourceFs,
					Sink:    service.sink,
					SrcPath: task.sourcePath,
					DstPath: destinationWebPPath,
					RelPath: task.relativePath,
					SrcInfo: task.info,
					Opts: assets.CopyOptions{
						Compress:     service.cfg.ShouldCompressImages,
						MinifySVGs:   service.cfg.ShouldMinifySVGs,
						KeepOriginal: false,
						CacheDir:     service.cfg.CacheDir + "/images",
						WebPQuality:  service.cfg.WebPQuality,
						Metrics:      service.metrics,
						OnWrite:      service.renderer.RegisterFile,
						ImageWorkers: service.cfg.ImageWorkers,
						Scheduler: func() scheduler.BuildScheduler {
							if service.ctx != nil {
								return service.ctx.Scheduler
							}
							return nil
						}(),
					},
				}

				syncContextInstance.imageTaskMu.Lock()
				syncContextInstance.imageTasks = append(syncContextInstance.imageTasks, imageCopyTask{task: task, opts: imageOptions})
				syncContextInstance.imageTaskMu.Unlock()
			}
			return
		}
		assetChan <- task
	}
}

type discoveryWalkOptions struct {
	walkerWg        *sync.WaitGroup
	walkConcurrency int
	debugAssets     bool
	syncCtx         *syncContext
	enqueue         func(assetTask)
}

func (service *assetService) setupDiscoveryWalk(walkOptions discoveryWalkOptions) func(context.Context, string, bool) error {
	if walkOptions.walkerWg == nil {
		panic("setupDiscoveryWalk: walkerWg is nil")
	}
	if walkOptions.syncCtx == nil {
		panic("setupDiscoveryWalk: syncCtx is nil")
	}
	if walkOptions.enqueue == nil {
		panic("setupDiscoveryWalk: enqueue is nil")
	}
	if walkOptions.walkConcurrency <= 0 {
		walkOptions.walkConcurrency = defaultWalkConcurrency
	}

	return func(ctx context.Context, dir string, isSite bool) error {
		exists, _ := afero.Exists(service.sourceFs, dir)
		if !exists {
			return nil
		}
		walkOptions.walkerWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    service.logger,
			Operation: "asset discovery walk",
			Fn: func() error {
				_ = fspkg.ParallelWalk(fspkg.WalkOptions{
					Ctx:         ctx,
					SourceFs:    service.sourceFs,
					Root:        dir,
					Concurrency: walkOptions.walkConcurrency,
					WalkFn: func(path string, info fs.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						if walkOptions.debugAssets {
							if isSite {
								atomic.AddInt64(&walkOptions.syncCtx.siteFiles, 1)
							} else {
								atomic.AddInt64(&walkOptions.syncCtx.themeFiles, 1)
							}
						}
						if filepath.Base(path) == "search.wasm" {
							return nil
						}

						// Incremental skip based on manifest
						if service.manifest != nil {
							if entry, ok := service.manifest.Get(path); ok {
								if entry.Size == info.Size() && entry.ModTime == info.ModTime().UnixNano() {
									relative, _ := fspkg.SafeRel(dir, path)
									fullRelativePath := "static/" + relative
									destPath := filepath.Join(service.cfg.OutputDir, fullRelativePath)
									if _, err := service.sink.Stat(destPath); err == nil {
										if service.renderer != nil {
											service.renderer.RegisterFile(destPath)
										}
										if service.metrics != nil {
											service.metrics.IncrementAssetsProcessed()
										}
										return nil
									}
								}
							}
						}

						relative, relativeErrorInstance := fspkg.SafeRel(dir, path)
						if relativeErrorInstance != nil || relative == "" {
							relative = service.handleRelPathManualFallback(dir, path, walkOptions.debugAssets, walkOptions.syncCtx)
							if relative == "" {
								return nil
							}
						}
						fullRelativePath := "static/" + relative

						if _, loaded := walkOptions.syncCtx.seen.LoadOrStore(fullRelativePath, true); !loaded {
							if walkOptions.debugAssets {
								service.recordDiscoverySample(isSite, fullRelativePath, walkOptions.syncCtx)
							}
							walkOptions.enqueue(assetTask{sourcePath: path, relativePath: fullRelativePath, info: info})
						}
						return nil
					},
				})
				return nil
			},
			Cleanup: walkOptions.walkerWg.Done,
		})
		return nil
	}
}

func (service *assetService) handleRelPathManualFallback(dir, path string, debugAssets bool, syncContextInstance *syncContext) string {
	baseNormalized := fspkg.NormalizePath(dir)
	pathNormalized := fspkg.NormalizePath(path)
	if !fspkg.IsPathInOrSame(pathNormalized, baseNormalized) {
		if debugAssets {
			atomic.AddInt64(&syncContextInstance.relErrs, 1)
		}
		return ""
	}
	relative := strings.TrimPrefix(pathNormalized, baseNormalized)
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" {
		if debugAssets {
			atomic.AddInt64(&syncContextInstance.relErrs, 1)
		}
		return ""
	}
	return relative
}

func (service *assetService) recordDiscoverySample(isSite bool, fullRelativePath string, syncContextInstance *syncContext) {
	if isSite {
		atomic.AddInt64(&syncContextInstance.siteEnqueued, 1)
	} else {
		atomic.AddInt64(&syncContextInstance.themeEnqueued, 1)
	}
	syncContextInstance.sampleMu.Lock()
	if isSite && len(syncContextInstance.siteSamples) < discoverySampleLimit {
		syncContextInstance.siteSamples = append(syncContextInstance.siteSamples, fullRelativePath)
	} else if !isSite && len(syncContextInstance.themeSamples) < discoverySampleLimit {
		syncContextInstance.themeSamples = append(syncContextInstance.themeSamples, fullRelativePath)
	}
	syncContextInstance.sampleMu.Unlock()
}

func (service *assetService) discoverContentAssets(ctx context.Context, syncContextInstance *syncContext, enqueue func(assetTask)) error {
	select {
	case contentAssets, ok := <-service.contentAssetsChan:
		if ok && contentAssets != nil {
			for _, asset := range contentAssets {
				relative, _ := fspkg.SafeRel(service.cfg.ContentDir, asset.Path)
				if _, loaded := syncContextInstance.seen.LoadOrStore(relative, true); !loaded {
					enqueue(assetTask{sourcePath: asset.Path, relativePath: relative, info: asset.Info})
				}
			}
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (service *assetService) logDiscoveryStats(dirs []string, syncCtx *syncContext) {
	service.logger.Info("Static discovery stats",
		"dirs", dirs,
		"site_files", atomic.LoadInt64(&syncCtx.siteFiles),
		"theme_files", atomic.LoadInt64(&syncCtx.themeFiles),
		"site_enqueued", atomic.LoadInt64(&syncCtx.siteEnqueued),
		"theme_enqueued", atomic.LoadInt64(&syncCtx.themeEnqueued),
		"rel_errors", atomic.LoadInt64(&syncCtx.relErrs),
		"site_samples", syncCtx.siteSamples,
		"theme_samples", syncCtx.themeSamples,
	)
}

func (service *assetService) copyCriticalAssets() {
	if service.cfg.Logo != "" {
		if err := service.copyFileOrLink(service.cfg.Logo, service.cfg.Logo); err != nil {
			if _, loaded := service.warnOnce.LoadOrStore("logo:"+service.cfg.Logo, true); !loaded {
				service.logger.Warn("Failed to copy logo", "src", service.cfg.Logo, "error", err)
			}
		}
	}
}
