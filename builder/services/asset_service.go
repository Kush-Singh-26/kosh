package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/utils"
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

	errChan := make(chan error, 10) // Increased capacity for multiple error sources

	// 1. Static Copy (excluding source CSS/JS handled by esbuild)
	go func() {
		defer wg.Done()

		// Check for early cancellation
		select {
		case <-ctx.Done():
			s.logger.Warn("Asset build cancelled", "reason", ctx.Err())
			return
		default:
		}

		// Theme Static
		exists, err := afero.Exists(s.sourceFs, s.cfg.StaticDir)
		if err != nil {
			s.logger.Warn("Failed to check theme static dir", "path", s.cfg.StaticDir, "error", err)
		}
		if exists {
			// Exclude .css and .js files from raw copy (they're handled by esbuild)
			destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
			if err := utils.CopyDirVFS(ctx, s.sourceFs, s.destFs, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, []string{".css", ".js"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality); err != nil {
				s.logger.Warn("Failed to copy theme static assets", "error", err)
				errChan <- err
				return
			}
		}

		// Check for cancellation between operations
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Site Static (Root 'static' folder)
		exists, err = afero.Exists(s.sourceFs, "static")
		if err != nil {
			s.logger.Warn("Failed to check site static dir", "path", "static", "error", err)
		}
		if exists {
			destStaticDir := filepath.Join(s.cfg.OutputDir, "static")
			if err := utils.CopyDirVFS(ctx, s.sourceFs, s.destFs, "static", destStaticDir, s.cfg.CompressImages, []string{".css", ".js"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality); err != nil {
				s.logger.Warn("Failed to copy site static assets", "error", err)
				errChan <- err
				return
			}
		}

		// Copy wasm_exec.js separately (it's needed by the WASM search but shouldn't be processed by esbuild)
		wasmExecPath := filepath.Join(s.cfg.StaticDir, "js/wasm_exec.js")
		exists, err = afero.Exists(s.sourceFs, wasmExecPath)
		if err != nil {
			s.logger.Warn("Failed to check wasm_exec.js", "path", wasmExecPath, "error", err)
		}
		if exists {
			err := func() error {
				src, err := s.sourceFs.Open(wasmExecPath)
				if err != nil {
					return err
				}
				defer func() { _ = src.Close() }()
				wasmExecDestPath := filepath.Join(s.cfg.OutputDir, "static/js/wasm_exec.js")
				if err := s.destFs.MkdirAll(filepath.Dir(wasmExecDestPath), 0755); err != nil {
					s.logger.Warn("Failed to create wasm_exec.js directory", "path", filepath.Dir(wasmExecDestPath), "error", err)
				}
				dest, err := s.destFs.Create(wasmExecDestPath)
				if err != nil {
					return err
				}
				defer func() { _ = dest.Close() }()
				if _, err := io.Copy(dest, src); err != nil {
					s.logger.Warn("Failed to copy wasm_exec.js", "path", wasmExecDestPath, "error", err)
					return err
				}
				s.renderer.RegisterFile(wasmExecDestPath)
				return nil
			}()
			if err != nil {
				errChan <- err
				return
			}
		}

		// Copy wasm_engine.js separately
		wasmEngineSitePath := filepath.Join("static", "js", "wasm_engine.js")
		wasmEngineThemePath := filepath.Join(s.cfg.StaticDir, "js", "wasm_engine.js")
		var wasmEngineSourcePath string
		exists, err = afero.Exists(s.sourceFs, wasmEngineSitePath)
		if err != nil {
			s.logger.Warn("Failed to check wasm_engine.js site path", "path", wasmEngineSitePath, "error", err)
		}
		if exists {
			wasmEngineSourcePath = wasmEngineSitePath
		} else {
			exists, err = afero.Exists(s.sourceFs, wasmEngineThemePath)
			if err != nil {
				s.logger.Warn("Failed to check wasm_engine.js theme path", "path", wasmEngineThemePath, "error", err)
			}
			if exists {
				wasmEngineSourcePath = wasmEngineThemePath
			}
		}
		if wasmEngineSourcePath != "" {
			err := func() error {
				src, err := s.sourceFs.Open(wasmEngineSourcePath)
				if err != nil {
					return err
				}
				defer func() { _ = src.Close() }()
				wasmEngineDestPath := filepath.Join(s.cfg.OutputDir, "static/js/wasm_engine.js")
				if err := s.destFs.MkdirAll(filepath.Dir(wasmEngineDestPath), 0755); err != nil {
					s.logger.Warn("Failed to create wasm_engine.js directory", "path", filepath.Dir(wasmEngineDestPath), "error", err)
				}
				dest, err := s.destFs.Create(wasmEngineDestPath)
				if err != nil {
					return err
				}
				defer func() { _ = dest.Close() }()
				if _, err := io.Copy(dest, src); err != nil {
					s.logger.Warn("Failed to copy wasm_engine.js", "path", wasmEngineDestPath, "error", err)
					return err
				}
				s.renderer.RegisterFile(wasmEngineDestPath)
				return nil
			}()
			if err != nil {
				errChan <- err
				return
			}
		}

		// Copy engine.js separately
		engineSitePath := filepath.Join("static", "wasm", "engine.js")
		engineThemePath := filepath.Join(s.cfg.StaticDir, "wasm", "engine.js")
		var engineSourcePath string
		exists, err = afero.Exists(s.sourceFs, engineSitePath)
		if err != nil {
			s.logger.Warn("Failed to check engine.js site path", "path", engineSitePath, "error", err)
		}
		if exists {
			engineSourcePath = engineSitePath
		} else {
			exists, err = afero.Exists(s.sourceFs, engineThemePath)
			if err != nil {
				s.logger.Warn("Failed to check engine.js theme path", "path", engineThemePath, "error", err)
			}
			if exists {
				engineSourcePath = engineThemePath
			}
		}
		if engineSourcePath != "" {
			err := func() error {
				src, err := s.sourceFs.Open(engineSourcePath)
				if err != nil {
					return err
				}
				defer func() { _ = src.Close() }()
				engineDestPath := filepath.Join(s.cfg.OutputDir, "static", "wasm", "engine.js")
				if err := s.destFs.MkdirAll(filepath.Dir(engineDestPath), 0755); err != nil {
					s.logger.Warn("Failed to create engine.js directory", "path", filepath.Dir(engineDestPath), "error", err)
				}
				dest, err := s.destFs.Create(engineDestPath)
				if err != nil {
					return err
				}
				defer func() { _ = dest.Close() }()
				if _, err := io.Copy(dest, src); err != nil {
					s.logger.Warn("Failed to copy engine.js", "path", engineDestPath, "error", err)
					return err
				}
				s.renderer.RegisterFile(engineDestPath)
				return nil
			}()
			if err != nil {
				errChan <- err
				return
			}
		}

		// WASM Search Engine Fallback logic
		wasmSitePath := filepath.Join("static", "wasm", "search.wasm")
		wasmThemePath := filepath.Join(s.cfg.StaticDir, "wasm", "search.wasm")
		wasmDestPath := filepath.Join(s.cfg.OutputDir, "static", "wasm", "search.wasm")

		var wasmSourcePath string
		exists, err = afero.Exists(s.sourceFs, wasmSitePath)
		if err != nil {
			s.logger.Warn("Failed to check search.wasm site path", "path", wasmSitePath, "error", err)
		}
		if exists {
			wasmSourcePath = wasmSitePath
		} else {
			exists, err = afero.Exists(s.sourceFs, wasmThemePath)
			if err != nil {
				s.logger.Warn("Failed to check search.wasm theme path", "path", wasmThemePath, "error", err)
			}
			if exists {
				wasmSourcePath = wasmThemePath
			}
		}

		if wasmSourcePath != "" {
			err := func() error {
				src, err := s.sourceFs.Open(wasmSourcePath)
				if err != nil {
					return err
				}
				defer func() { _ = src.Close() }()
				if err := s.destFs.MkdirAll(filepath.Dir(wasmDestPath), 0755); err != nil {
					s.logger.Warn("Failed to create search.wasm directory", "path", filepath.Dir(wasmDestPath), "error", err)
				}
				dest, err := s.destFs.Create(wasmDestPath)
				if err != nil {
					return err
				}
				defer func() { _ = dest.Close() }()
				if _, err := io.Copy(dest, src); err != nil {
					s.logger.Warn("Failed to copy search.wasm", "path", wasmDestPath, "error", err)
					return err
				}
				s.renderer.RegisterFile(wasmDestPath)
				return nil
			}()
			if err != nil {
				errChan <- err
				return
			}
		}

		// Ensure Site Logo is copied exactly
		if s.cfg.Logo != "" {
			if exists, _ := afero.Exists(s.sourceFs, s.cfg.Logo); exists {
				err := func() error {
					src, err := s.sourceFs.Open(s.cfg.Logo)
					if err != nil {
						return err
					}
					defer func() { _ = src.Close() }()
					destPath := filepath.Join(s.cfg.OutputDir, s.cfg.Logo)
					if err := s.destFs.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
						s.logger.Warn("Failed to create logo directory", "path", filepath.Dir(destPath), "error", err)
					}
					dest, err := s.destFs.Create(destPath)
					if err != nil {
						return err
					}
					defer func() { _ = dest.Close() }()
					if _, err := io.Copy(dest, src); err != nil {
						s.logger.Warn("Failed to copy logo", "path", destPath, "error", err)
						return err
					}
					s.renderer.RegisterFile(destPath)
					return nil
				}()
				if err != nil {
					errChan <- err
					return
				}
			}
		}

		// Site Content Assets (Colocated images/files)
		exists, err = afero.Exists(s.sourceFs, s.cfg.ContentDir)
		if err != nil {
			s.logger.Warn("Failed to check content dir", "path", s.cfg.ContentDir, "error", err)
		}
		if exists {
			// Copy everything from content/ to outputDir, excluding .md files
			// This preserves the directory structure for colocated assets
			if err := utils.CopyDirVFS(ctx, s.sourceFs, s.destFs, s.cfg.ContentDir, s.cfg.OutputDir, s.cfg.CompressImages, []string{".md"}, s.renderer.RegisterFile, s.cfg.CacheDir+"/images", s.cfg.ImageWorkers, s.cfg.WebPQuality); err != nil {
				s.logger.Warn("Failed to copy content assets", "error", err)
				errChan <- err
				return
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
		// Use hash-based check even in dev mode to avoid redundant esbuild runs
		// force is now only true if explicitly requested via some other mechanism (currently false)
		force := false
		assets, assetErr := utils.BuildAssetsEsbuild(s.sourceFs, s.destFs, s.cfg.StaticDir, destStaticDir, s.cfg.CompressImages, s.renderer.RegisterFile, s.cfg.CacheDir+"/assets", force)
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
