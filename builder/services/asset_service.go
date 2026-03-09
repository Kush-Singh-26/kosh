package services

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

type assetServiceImpl struct {
	sourceFs      afero.Fs
	sink          utils.ArtifactSink
	cfg           *config.Config
	renderer      RenderService
	logger        *slog.Logger
	metrics       *metrics.BuildMetrics
	contentAssets []ScannedAsset
	assetsReady   chan struct{}
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

func (s *assetServiceImpl) SetSink(sink utils.ArtifactSink)        { s.sink = sink }
func (s *assetServiceImpl) SetSourceFs(fs afero.Fs)                { s.sourceFs = fs }
func (s *assetServiceImpl) SetMetrics(m *metrics.BuildMetrics)     { s.metrics = m }
func (s *assetServiceImpl) SetAssetsReadySignal(ch chan struct{})  { s.assetsReady = ch }
func (s *assetServiceImpl) SetContentAssets(assets []ScannedAsset) { s.contentAssets = assets }

func (s *assetServiceImpl) Build(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)
	utils.SetGlobalImageProcessingLimit(s.cfg.ImageWorkers)

	destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
	var m interface {
		RecordImageOptimization(original, optimized int64)
		RecordImageResizeSkipped()
	}
	if s.metrics != nil {
		m = s.metrics
	}

	// 1. Static Copy
	g.Go(func() error {
		copyTimer := utils.StartPhase("Asset copy root/static")
		defer copyTimer.Stop()

		themeDir := s.cfg.StaticDir
		if themeDir == "" {
			themeDir = "themes/blog/static"
		}

		// A) Theme Static
		g.Go(func() error {
			exists, _ := afero.Exists(s.sourceFs, themeDir)
			if exists {
				return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, themeDir, destStaticDir, s.cfg.CompressImages, []string{".css", ".js", ".br"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, m)
			}
			return nil
		})

		// B) Site Static (Root)
		if themeDir != "static" {
			g.Go(func() error {
				exists, _ := afero.Exists(s.sourceFs, "static")
				if exists {
					return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, "static", destStaticDir, s.cfg.CompressImages, []string{".css", ".js", ".br"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, m)
				}
				return nil
			})
		}

		// C) Content Images
		g.Go(func() error {
			if len(s.contentAssets) > 0 {
				return s.copyContentAssetsFromManifest()
			}
			return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, s.cfg.ContentDir, s.cfg.OutputDir, s.cfg.CompressImages, []string{".md"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, m)
		})

		s.copyCriticalAssets()
		return nil
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

func (s *assetServiceImpl) copyContentAssetsFromManifest() error {
	numWorkers := s.cfg.ImageWorkers
	if numWorkers <= 0 {
		numWorkers = 8
	}

	p, pCtx := errgroup.WithContext(context.Background())
	p.SetLimit(numWorkers)

	for _, asset := range s.contentAssets {
		a := asset
		p.Go(func() error {
			rel, _ := utils.SafeRel(s.cfg.ContentDir, a.Path)
			dst := filepath.Join(s.cfg.OutputDir, rel)
			return utils.CopyFileWithOptionalImageProcessing(pCtx, s.sourceFs, s.sink, a.Path, dst, s.cfg.CompressImages, s.cfg.CacheDir+"/images", s.cfg.WebPQuality, a.Info, s.metrics, s.renderer.RegisterFile)
		})
	}
	return p.Wait()
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
	destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
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
