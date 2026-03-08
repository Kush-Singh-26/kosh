package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)


func (s *assetServiceImpl) copyFileOrLink(src, dst string) error {
	srcFile, err := s.sourceFs.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	if err := s.sink.MkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}

	errWrite := s.sink.WriteStream(dst, func(w io.Writer) error {
		_, err := io.Copy(w, srcFile)
		return err
	})
	if errWrite == nil {
		s.renderer.RegisterFile(dst)
		// Preserve mtime to enable future hardlink hits
		if info, err := srcFile.Stat(); err == nil {
			_ = s.sink.SetMtime(dst, info.ModTime())
		}
	}
	return errWrite
}

type assetServiceImpl struct {
	sourceFs afero.Fs
	sink     utils.ArtifactSink
	cfg      *config.Config
	renderer RenderService
	logger   *slog.Logger
	metrics  *metrics.BuildMetrics
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

func (s *assetServiceImpl) SetSink(sink utils.ArtifactSink) {
	s.sink = sink
}

func (s *assetServiceImpl) SetSourceFs(fs afero.Fs) {
	s.sourceFs = fs
}

func (s *assetServiceImpl) SetMetrics(m *metrics.BuildMetrics) {
	s.metrics = m
}

func (s *assetServiceImpl) Build(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(2)

	errChan := make(chan error, 10) // Increased capacity for multiple error sources

	// 1. Static Copy (excluding source CSS/JS handled by esbuild)
	// All three CopyDirVFS calls run concurrently via errgroup for maximum I/O overlap.
	go func() {
		defer wg.Done()

		// Check for early cancellation
		select {
		case <-ctx.Done():
			s.logger.Warn("Asset build cancelled", "reason", ctx.Err())
			return
		default:
		}

		destStaticDir := filepath.Join(s.cfg.OutputDir, "static")

		// Defense against typed nil interface
		var metrics interface {
			RecordImageOptimization(original, optimized int64)
			RecordImageResizeSkipped()
		}
		if s.metrics != nil {
			metrics = s.metrics
		}

		// Run all CopyDirVFS calls concurrently
		g, gCtx := errgroup.WithContext(ctx)
		utils.SetGlobalImageProcessingLimit(s.cfg.ImageWorkers)
		copyThemeTimer := utils.StartPhase("Asset copy theme/static")
		copyContentTimer := utils.StartPhase("Asset copy content")
		copySiteStaticTimer := utils.StartPhase("Asset copy root/static")

		themeExists, err := afero.Exists(s.sourceFs, s.cfg.StaticDir)
		if err != nil {
			s.logger.Warn("Failed to check theme static dir", "path", s.cfg.StaticDir, "error", err)
		}

		// A) Theme Static
		g.Go(func() error {
			defer copyThemeTimer.Stop()
			if themeExists {
				// Exclude .css/.js (handled by esbuild) and .gz/.br (pre-compressed
				// variants written by CheckWASMFs belong only in the output dir —
				// never copy them from the source tree into output).
				return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, []string{".css", ".js", ".gz", ".br"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, metrics)
			}
			// If theme not found, try default static dir
			if err := utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, "static", destStaticDir, s.cfg.CompressImages, []string{".css", ".js", ".gz", ".br"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, metrics); err != nil {
				// Non-fatal if static doesn't exist
				s.logger.Warn("Static directory not found", "dir", "static")
			}
			return nil
		})

		// B) Content Images (collocated images)
		g.Go(func() error {
			defer copyContentTimer.Stop()
			return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, s.cfg.ContentDir, s.cfg.OutputDir, s.cfg.CompressImages, []string{".md"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, metrics)
		})

		// C) Site Static (Root 'static' folder) — only if it differs from theme static dir
		if s.cfg.StaticDir != "static" {
			siteStaticExists, err := afero.Exists(s.sourceFs, "static")
			if err != nil {
				s.logger.Warn("Failed to check site static dir", "path", "static", "error", err)
			}
			if siteStaticExists {
				g.Go(func() error {
					defer copySiteStaticTimer.Stop()
					return utils.CopyDirVFS(gCtx, s.sourceFs, s.sink, "static", destStaticDir, s.cfg.CompressImages, []string{".css", ".js", ".gz", ".br"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality, metrics)
				})
			} else {
				copySiteStaticTimer.Stop()
			}
		} else {
			copySiteStaticTimer.Stop()
		}

		if err := g.Wait(); err != nil {
			errChan <- err
			return
		}

		// Post-copy: Ensure critical assets are copied exactly (after parallel copies complete)

		// Ensure Site Logo is copied exactly (ignoring global compression for this critical asset)
		if s.cfg.Logo != "" {
			absLogo, _ := filepath.Abs(s.cfg.Logo)
			if exists, _ := afero.Exists(s.sourceFs, absLogo); exists {
				if err := s.copyFileOrLink(absLogo, s.cfg.Logo); err != nil {
					s.logger.Warn("Failed to explicitly copy site logo", "path", s.cfg.Logo, "error", err)
				}
			}
		}

		// Also ensure favicon.png from theme is copied exactly if it exists
		faviconPath := filepath.Join(s.cfg.StaticDir, "images/favicon.png")
		if exists, _ := afero.Exists(s.sourceFs, faviconPath); exists {
			destFavicon := "static/images/favicon.png"
			if err := s.copyFileOrLink(faviconPath, destFavicon); err != nil {
				s.logger.Warn("Failed to explicitly copy favicon", "path", faviconPath, "error", err)
			}
		}
	}()

	// 2. Esbuild Bundling (CSS/JS)
	go func() {
		defer wg.Done()

		// Check for early cancellation
		select {
		case <-ctx.Done():
			s.logger.Warn("Asset bundling cancelled", "reason", ctx.Err())
			return
		default:
		}

		destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
		esbuildTimer := utils.StartPhase("Asset esbuild")
		defer esbuildTimer.Stop()
		// Use hash-based check even in dev mode to avoid redundant esbuild runs
		// force is now only true if explicitly requested via some other mechanism (currently false)
		force := false
		assets, assetErr := utils.BuildAssetsEsbuild(s.sourceFs, s.sink, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force)
		if assetErr != nil {
			s.logger.Error("Failed to build assets", "error", assetErr)
			errChan <- assetErr
			return
		}
		s.renderer.SetAssets(assets)
	}()

	// Wait for both goroutines or context cancellation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		s.logger.Warn("Asset build interrupted", "reason", ctx.Err())
		return ctx.Err()
	case err := <-errChan:
		return err
	case <-done:
		return nil
	}
}
