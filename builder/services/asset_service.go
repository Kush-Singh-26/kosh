package services

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

type assetServiceImpl struct {
	sourceFs          afero.Fs
	sink              utils.ArtifactSink
	cfg               *config.Config
	renderer          RenderService
	logger            *slog.Logger
	metrics           *metrics.BuildMetrics
	contentAssetsChan <-chan []models.ScannedAsset
	contentAssets     []models.ScannedAsset
	assetsReady       chan struct{}
}

func NewAssetService(sourceFs afero.Fs, sink utils.ArtifactSink, cfg *config.Config, renderer RenderService, logger *slog.Logger) AssetService {
	return &assetServiceImpl{
		sourceFs: sourceFs,
		sink:     sink,
		cfg:      cfg,
		renderer: renderer,
		logger:   logger,
	}
}

func (s *assetServiceImpl) ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

func (s *assetServiceImpl) SetMetrics(m *metrics.BuildMetrics)    { s.metrics = m }
func (s *assetServiceImpl) SetAssetsReadySignal(ch chan struct{}) { s.assetsReady = ch }
func (s *assetServiceImpl) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	s.contentAssetsChan = ch
}

func (s *assetServiceImpl) Build(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// 1. Unified Asset Copy Phase
	g.Go(func() error {
		copyTimer := utils.StartPhase("Asset copy unified")
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
				_ = utils.ParallelWalk(dCtx, s.sourceFs, themeDir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					if filepath.Base(path) == "search.wasm" {
						return nil
					}
					rel, _ := utils.SafeRel(themeDir, path)
					assetSyncMap.Store("static/"+rel, assetTask{srcPath: path, info: info})
					return nil
				})
			}
			return nil
		})

		if themeDir != "static" {
			discoveryGroup.Go(func() error {
				exists, _ := afero.Exists(s.sourceFs, "static")
				if exists {
					_ = utils.ParallelWalk(dCtx, s.sourceFs, "static", walkConcurrency, func(path string, info fs.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						if filepath.Base(path) == "search.wasm" {
							return nil
						}
						rel, _ := utils.SafeRel("static", path)
						assetSyncMap.Store("static/"+rel, assetTask{srcPath: path, info: info})
						return nil
					})
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
			rel, _ := utils.SafeRel(s.cfg.ContentDir, a.Path)
			assetMap[rel] = assetTask{srcPath: a.Path, info: a.Info}
		}

		// Dispatch with STRICT limit (128 for Windows to avoid I/O contention)
		copyGroup, copyCtx := errgroup.WithContext(gCtx)
		copyGroup.SetLimit(128)

		for rel, task := range assetMap {
			r, t := rel, task
			copyGroup.Go(func() error {
				dst := filepath.Join(s.cfg.OutputDir, r)
				opts := utils.CopyOptions{
					Compress:     s.cfg.CompressImages,
					CacheDir:     s.cfg.CacheDir + "/images",
					WebPQuality:  s.cfg.WebPQuality,
					Metrics:      s.metrics,
					OnWrite:      s.renderer.RegisterFile,
					ImageWorkers: s.cfg.ImageWorkers,
				}
				return utils.CopyFileWithOptionalImageProcessing(copyCtx, s.sourceFs, s.sink, t.srcPath, dst, t.info, opts)
			})
		}

		s.copyCriticalAssets()
		return copyGroup.Wait()
	})

	// 2. Esbuild
	g.Go(func() error {
		esbuildTimer := utils.StartPhase("Asset esbuild")
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

func (s *assetServiceImpl) copyCriticalAssets() {
	if s.cfg.Logo != "" {
		_ = s.copyFileOrLink(s.cfg.Logo, s.cfg.Logo)
	}
	faviconPath := filepath.Join(s.cfg.StaticDir, "images/favicon.png")
	if exists, _ := afero.Exists(s.sourceFs, faviconPath); exists {
		_ = s.copyFileOrLink(faviconPath, "static/images/favicon.png")
	}
}

func (s *assetServiceImpl) copyFileOrLink(src, dst string) error {
	info, err := s.sourceFs.Stat(src)
	if err != nil {
		return err
	}
	return utils.CopyFileVFS(s.sourceFs, s.sink, src, dst, info.ModTime().UnixNano(), s.renderer.RegisterFile)
}

func (s *assetServiceImpl) buildEsbuildAssets(force bool) (map[string]string, error) {
	destStaticDir, _ := filepath.Abs(filepath.Join(s.cfg.OutputDir, "static"))
	assets, assetErr := utils.BuildAssetsEsbuild(s.sourceFs, s.sink, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force)
	if assetErr == nil {
		s.renderer.SetAssets(assets)
	}
	return assets, assetErr
}

func (s *assetServiceImpl) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	esbuildTimer := utils.StartPhase("Asset esbuild")
	defer esbuildTimer.Stop()
	return s.buildEsbuildAssets(true)
}
