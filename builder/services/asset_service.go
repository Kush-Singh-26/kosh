package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"

	"my-ssg/builder/config"
	"my-ssg/builder/utils"
)

type assetServiceImpl struct {
	sourceFs afero.Fs
	destFs   afero.Fs
	cfg      *config.Config
	renderer RenderService
	logger   *slog.Logger
}

func NewAssetService(sourceFs, destFs afero.Fs, cfg *config.Config, renderer RenderService, logger *slog.Logger) AssetService {
	return &assetServiceImpl{
		sourceFs: sourceFs,
		destFs:   destFs,
		cfg:      cfg,
		renderer: renderer,
		logger:   logger,
	}
}

func (s *assetServiceImpl) Build(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(2)

	// 1. Static Copy (excluding source CSS/JS handled by esbuild)
	go func() {
		defer wg.Done()
		// Theme Static
		if exists, _ := afero.Exists(s.sourceFs, s.cfg.StaticDir); exists {
			// Exclude .css and .js files from raw copy (they're handled by esbuild)
			destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
			if err := utils.CopyDirVFS(s.sourceFs, s.destFs, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, []string{".css", ".js"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers); err != nil {
				s.logger.Warn("Failed to copy theme static assets", "error", err)
			}
		}

		// Site Static (Root 'static' folder)
		if exists, _ := afero.Exists(s.sourceFs, "static"); exists {
			destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
			if err := utils.CopyDirVFS(s.sourceFs, s.destFs, "static", destStaticDir, s.cfg.CompressImages, []string{".css", ".js"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers); err != nil {
				s.logger.Warn("Failed to copy site static assets", "error", err)
			}
		}

		// Copy wasm_exec.js separately (it's needed by the WASM search but shouldn't be processed by esbuild)
		wasmExecPath := s.cfg.StaticDir + "/js/wasm_exec.js"
		if exists, _ := afero.Exists(s.sourceFs, wasmExecPath); exists {
			src, err := s.sourceFs.Open(wasmExecPath)
			if err == nil {
				defer src.Close()
				wasmExecDestPath := filepath.Join(s.cfg.OutputDir, "static/js/wasm_exec.js")
				_ = s.destFs.MkdirAll(filepath.Join(s.cfg.OutputDir, "static/js"), 0755)
				dest, err := s.destFs.Create(wasmExecDestPath)
				if err == nil {
					defer dest.Close()
					if _, err := io.Copy(dest, src); err == nil {
						s.renderer.RegisterFile(wasmExecDestPath)
					}
				}
			}
		}

		// WASM Search Engine Fallback logic
		// 1. Check site root: static/wasm/search.wasm
		// 2. Check theme: themes/<theme>/static/wasm/search.wasm
		wasmSitePath := "static/wasm/search.wasm"
		wasmThemePath := filepath.Join(s.cfg.StaticDir, "wasm/search.wasm")
		wasmDestPath := filepath.Join(s.cfg.OutputDir, "static/wasm/search.wasm")

		var wasmSourcePath string
		if exists, _ := afero.Exists(s.sourceFs, wasmSitePath); exists {
			wasmSourcePath = wasmSitePath
		} else if exists, _ := afero.Exists(s.sourceFs, wasmThemePath); exists {
			wasmSourcePath = wasmThemePath
		}

		if wasmSourcePath != "" {
			src, err := s.sourceFs.Open(wasmSourcePath)
			if err == nil {
				defer src.Close()
				_ = s.destFs.MkdirAll(filepath.Dir(wasmDestPath), 0755)
				dest, err := s.destFs.Create(wasmDestPath)
				if err == nil {
					defer dest.Close()
					if _, err := io.Copy(dest, src); err == nil {
						s.renderer.RegisterFile(wasmDestPath)
					}
				}
			}
		}

		// Ensure Site Logo is copied exactly (no WebP compression)
		if s.cfg.Logo != "" {
			if exists, _ := afero.Exists(s.sourceFs, s.cfg.Logo); exists {
				src, err := s.sourceFs.Open(s.cfg.Logo)
				if err == nil {
					defer src.Close()
					destPath := filepath.Join(s.cfg.OutputDir, s.cfg.Logo)
					_ = s.destFs.MkdirAll(filepath.Dir(destPath), 0755)
					dest, err := s.destFs.Create(destPath)
					if err == nil {
						defer dest.Close()
						if _, err := io.Copy(dest, src); err == nil {
							s.renderer.RegisterFile(destPath)
						}
					}
				}
			}
		}
	}()

	// 2. Esbuild Bundling (CSS/JS)
	go func() {
		defer wg.Done()
		destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
		assets, assetErr := utils.BuildAssetsEsbuild(s.sourceFs, s.destFs, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets")
		if assetErr != nil {
			s.logger.Error("Failed to build assets", "error", assetErr)
			return
		}
		s.renderer.SetAssets(assets)
	}()

	wg.Wait()
	return nil
}
