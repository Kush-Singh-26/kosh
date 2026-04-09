package asset

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
	srcPath string
	relPath string
	info    fs.FileInfo
}

type imageCopyTask struct {
	task assetTask
	opts assets.ProcessImageOptions
}

type syncContext struct {
	imageQueue []imageCopyTask
	seen       sync.Map

	imageQueueMu sync.Mutex // protects imageQueue

	siteFiles     int64
	themeFiles    int64
	siteEnqueued  int64
	themeEnqueued int64
	relErrs       int64

	siteSamples  []string
	themeSamples []string
	sampleMu     sync.Mutex // protects siteSamples and themeSamples
}

func (s *assetService) isWebPCandidate(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return false
	}

	// Exclude critical assets from WebP conversion to preserve compatibility
	// with non-img tags (icons, social sharing, etc.)
	base := strings.ToLower(filepath.Base(path))
	if base == "icon-192.png" || base == "icon-512.png" {
		return false
	}

	if s.cfg != nil && s.cfg.Logo != "" {
		logoBase := strings.ToLower(filepath.Base(s.cfg.Logo))
		if base == logoBase {
			return false
		}
	}

	return true
}

// syncStaticAssets discovers and copies all static assets to the sink synchronously.
func (s *assetService) syncStaticAssets(ctx context.Context, bgCtx context.Context, skipImages bool) ([]imageCopyTask, error) {
	themeDir, siteStaticDir := s.getStaticSourceDirs()
	debugAssets := os.Getenv("KOSH_DEBUG_ASSETS") == "1"

	sc := &syncContext{}

	numWorkers := s.cfg.ImageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	assetChan := make(chan assetTask, assetChanBuffer)
	copyGroup, copyCtx := errgroup.WithContext(ctx)
	copyGroup.SetLimit(assetCopyGroupLimit)

	workerWg := s.setupAssetWorker(assetChan, copyGroup, copyCtx)

	walkerWg := sync.WaitGroup{}
	discoveryGroup, dCtx := errgroup.WithContext(ctx)
	walkConcurrency := max(numWorkers/assetWalkConcurrencyDiv, assetMinWalkConcurrency)

	enqueue := s.setupImageEnqueue(bgCtx, skipImages, sc, assetChan)
	walkFunc := s.setupDiscoveryWalk(discoveryWalkOptions{
		walkerWg:        &walkerWg,
		walkConcurrency: walkConcurrency,
		debugAssets:     debugAssets,
		syncCtx:         sc,
		enqueue:         enqueue,
	})

	themeDirNorm := fspkg.NormalizePath(themeDir)
	siteStaticNorm := fspkg.NormalizePath(siteStaticDir)
	sameStatic := themeDirNorm == siteStaticNorm
	if runtime.GOOS == "windows" {
		sameStatic = strings.EqualFold(themeDirNorm, siteStaticNorm)
	}

	if !sameStatic {
		discoveryGroup.Go(func() error { return walkFunc(ctx, siteStaticDir, true) })
	}

	discoveryGroup.Go(func() error { return walkFunc(ctx, themeDir, false) })

	if s.contentAssetsChan != nil {
		discoveryGroup.Go(func() error {
			return s.discoverContentAssets(dCtx, sc, enqueue)
		})
	}

	err := discoveryGroup.Wait()
	walkerWg.Wait()
	close(assetChan)
	workerWg.Wait()

	if debugAssets {
		s.logDiscoveryStats(siteStaticDir, themeDir, sc)
	}

	if err != nil {
		return nil, err
	}

	s.copyCriticalAssets()

	if err := copyGroup.Wait(); err != nil {
		return nil, err
	}

	return sc.imageQueue, nil
}

func (s *assetService) getStaticSourceDirs() (string, string) {
	themeDir := s.cfg.StaticDir
	if themeDir == "" {
		themeDir = "themes/blog/static"
	}
	siteStaticDir := "static"
	if s.cfg.SiteRoot != "" {
		siteStaticDir = filepath.Join(s.cfg.SiteRoot, "static")
	}
	return themeDir, siteStaticDir
}

func (s *assetService) setupAssetWorker(assetChan <-chan assetTask, group *errgroup.Group, ctx context.Context) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    s.logger,
		Operation: "asset worker",
		Fn: func() error {
			for task := range assetChan {
				t := task
				group.Go(func() error {
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
						Ctx:     ctx,
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
			return nil
		},
		Cleanup: wg.Done,
	})
	return wg
}

func (s *assetService) setupImageEnqueue(bgCtx context.Context, skipImages bool, sc *syncContext, assetChan chan<- assetTask) func(assetTask) {
	return func(t assetTask) {
		if s.cfg.CompressImages && s.isWebPCandidate(t.srcPath) {
			relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(t.relPath), "/")
			relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
			assets.RecordConvertedImage(relSrc, relDst)
			assets.RecordConvertedImage(strings.TrimPrefix(relSrc, "/"), relDst)

			dst := filepath.Join(s.cfg.OutputDir, t.relPath)
			dstWebp := dst[:len(dst)-len(filepath.Ext(dst))] + ".webp"

			if skipImages && s.cfg.IsDev {
				if _, err := s.sink.Stat(dstWebp); err == nil {
					s.sink.Register(dstWebp)
					if s.renderer != nil {
						s.renderer.RegisterFile(dstWebp)
					}
					return
				}
			}

			err := assets.CopyFromDiskCache(assets.CopyFromDiskCacheOptions{
				SrcFs:        s.sourceFs,
				Sink:         s.sink,
				RelPath:      t.relPath,
				SrcPath:      t.srcPath,
				DstPath:      dstWebp,
				CacheDir:     s.cfg.CacheDir + "/images",
				SrcInfo:      t.info,
				Metrics:      s.metrics,
				OnWrite:      s.renderer.RegisterFile,
				KeepOriginal: s.cfg.IsDev || s.cfg.Features.RawMarkdown,
				MuteMetrics:  skipImages,
			})
			if err == nil {
				return
			}
			if !errors.Is(err, assets.ErrCacheMiss) {
				if _, loaded := s.warnOnce.LoadOrStore("cache-fail:"+t.srcPath, true); !loaded {
					s.logger.Warn("Disk cache lookup failed", "path", t.srcPath, "error", err)
				}
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
			sc.imageQueueMu.Lock()
			sc.imageQueue = append(sc.imageQueue, imageCopyTask{task: t, opts: imgOpts})
			sc.imageQueueMu.Unlock()
			return
		}
		assetChan <- t
	}
}

type discoveryWalkOptions struct {
	walkerWg        *sync.WaitGroup
	walkConcurrency int
	debugAssets     bool
	syncCtx         *syncContext
	enqueue         func(assetTask)
}

func (s *assetService) setupDiscoveryWalk(opts discoveryWalkOptions) func(context.Context, string, bool) error {
	if opts.walkerWg == nil {
		panic("setupDiscoveryWalk: walkerWg is nil")
	}
	if opts.syncCtx == nil {
		panic("setupDiscoveryWalk: syncCtx is nil")
	}
	if opts.enqueue == nil {
		panic("setupDiscoveryWalk: enqueue is nil")
	}
	if opts.walkConcurrency <= 0 {
		opts.walkConcurrency = defaultWalkConcurrency
	}

	return func(ctx context.Context, dir string, isSite bool) error {
		exists, _ := afero.Exists(s.sourceFs, dir)
		if !exists {
			return nil
		}
		opts.walkerWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    s.logger,
			Operation: "asset discovery walk",
			Fn: func() error {
				_ = fspkg.ParallelWalk(fspkg.WalkOptions{
					Ctx:         ctx,
					SourceFs:    s.sourceFs,
					Root:        dir,
					Concurrency: opts.walkConcurrency,
					WalkFn: func(path string, info fs.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						if opts.debugAssets {
							if isSite {
								atomic.AddInt64(&opts.syncCtx.siteFiles, 1)
							} else {
								atomic.AddInt64(&opts.syncCtx.themeFiles, 1)
							}
						}
						if filepath.Base(path) == "search.wasm" {
							return nil
						}

						rel, relErr := fspkg.SafeRel(dir, path)
						if relErr != nil || rel == "" {
							rel = s.handleRelPathManualFallback(dir, path, opts.debugAssets, opts.syncCtx)
							if rel == "" {
								return nil
							}
						}
						fullRel := "static/" + rel
						if _, loaded := opts.syncCtx.seen.LoadOrStore(fullRel, true); !loaded {
							if opts.debugAssets {
								s.recordDiscoverySample(isSite, fullRel, opts.syncCtx)
							}
							opts.enqueue(assetTask{srcPath: path, relPath: fullRel, info: info})
						}
						return nil
					},
				})
				return nil
			},
			Cleanup: opts.walkerWg.Done,
		})
		return nil
	}
}

func (s *assetService) handleRelPathManualFallback(dir, path string, debugAssets bool, sc *syncContext) string {
	baseNorm := fspkg.NormalizePath(dir)
	pathNorm := fspkg.NormalizePath(path)
	if !fspkg.IsPathInOrSame(pathNorm, baseNorm) {
		if debugAssets {
			atomic.AddInt64(&sc.relErrs, 1)
		}
		return ""
	}
	rel := strings.TrimPrefix(pathNorm, baseNorm)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		if debugAssets {
			atomic.AddInt64(&sc.relErrs, 1)
		}
		return ""
	}
	return rel
}

func (s *assetService) recordDiscoverySample(isSite bool, fullRel string, sc *syncContext) {
	if isSite {
		atomic.AddInt64(&sc.siteEnqueued, 1)
	} else {
		atomic.AddInt64(&sc.themeEnqueued, 1)
	}
	sc.sampleMu.Lock()
	if isSite && len(sc.siteSamples) < discoverySampleLimit {
		sc.siteSamples = append(sc.siteSamples, fullRel)
	} else if !isSite && len(sc.themeSamples) < discoverySampleLimit {
		sc.themeSamples = append(sc.themeSamples, fullRel)
	}
	sc.sampleMu.Unlock()
}

func (s *assetService) discoverContentAssets(ctx context.Context, sc *syncContext, enqueue func(assetTask)) error {
	select {
	case contentAssets, ok := <-s.contentAssetsChan:
		if ok && contentAssets != nil {
			for _, a := range contentAssets {
				rel, _ := fspkg.SafeRel(s.cfg.ContentDir, a.Path)
				if _, loaded := sc.seen.LoadOrStore(rel, true); !loaded {
					enqueue(assetTask{srcPath: a.Path, relPath: rel, info: a.Info})
				}
			}
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (s *assetService) logDiscoveryStats(siteStaticDir, themeDir string, sc *syncContext) {
	s.logger.Info("Static discovery stats",
		"site_dir", siteStaticDir,
		"theme_dir", themeDir,
		"site_files", atomic.LoadInt64(&sc.siteFiles),
		"theme_files", atomic.LoadInt64(&sc.themeFiles),
		"site_enqueued", atomic.LoadInt64(&sc.siteEnqueued),
		"theme_enqueued", atomic.LoadInt64(&sc.themeEnqueued),
		"rel_errors", atomic.LoadInt64(&sc.relErrs),
		"site_samples", sc.siteSamples,
		"theme_samples", sc.themeSamples,
	)
}

func (s *assetService) copyCriticalAssets() {
	if s.cfg.Logo != "" {
		if err := s.copyFileOrLink(s.cfg.Logo, s.cfg.Logo); err != nil {
			if _, loaded := s.warnOnce.LoadOrStore("logo:"+s.cfg.Logo, true); !loaded {
				s.logger.Warn("Failed to copy logo", "src", s.cfg.Logo, "error", err)
			}
		}
	}
}
