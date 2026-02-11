package run

import (
	"io"
	"path/filepath"
	"sync"

	"my-ssg/builder/utils"

	"github.com/spf13/afero"
)

func (b *Builder) copyStaticAndBuildAssets() {
	cfg := b.cfg
	var wg sync.WaitGroup
	wg.Add(2)

	// 1. Static Copy (excluding source CSS/JS handled by esbuild)
	go func() {
		defer wg.Done()
		// Theme Static
		if exists, _ := afero.Exists(b.SourceFs, cfg.StaticDir); exists {
			// Exclude .css and .js files from raw copy (they're handled by esbuild)
			if err := utils.CopyDirVFS(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, []string{".css", ".js"}, b.rnd.RegisterFile, ".kosh-cache/images"); err != nil {
				b.logger.Warn("Failed to copy theme static assets", "error", err)
			}
		}

		// Site Static (Root 'static' folder)
		if exists, _ := afero.Exists(b.SourceFs, "static"); exists {
			if err := utils.CopyDirVFS(b.SourceFs, b.DestFs, "static", "public/static", cfg.CompressImages, []string{".css", ".js"}, b.rnd.RegisterFile, ".kosh-cache/images"); err != nil {
				b.logger.Warn("Failed to copy site static assets", "error", err)
			}
		}

		// Copy wasm_exec.js separately (it's needed by the WASM search but shouldn't be processed by esbuild)
		wasmExecPath := cfg.StaticDir + "/js/wasm_exec.js"
		if exists, _ := afero.Exists(b.SourceFs, wasmExecPath); exists {
			src, err := b.SourceFs.Open(wasmExecPath)
			if err == nil {
				defer src.Close()
				_ = b.DestFs.MkdirAll("public/static/js", 0755)
				dest, err := b.DestFs.Create("public/static/js/wasm_exec.js")
				if err == nil {
					defer dest.Close()
					if _, err := io.Copy(dest, src); err == nil {
						b.rnd.RegisterFile("public/static/js/wasm_exec.js")
					}
				}
			}
		}

		// WASM Search Engine Fallback logic
		// 1. Check site root: static/wasm/search.wasm
		// 2. Check theme: themes/<theme>/static/wasm/search.wasm
		wasmSitePath := "static/wasm/search.wasm"
		wasmThemePath := filepath.Join(cfg.StaticDir, "wasm/search.wasm")
		wasmDestPath := "public/static/wasm/search.wasm"

		var wasmSourcePath string
		if exists, _ := afero.Exists(b.SourceFs, wasmSitePath); exists {
			wasmSourcePath = wasmSitePath
		} else if exists, _ := afero.Exists(b.SourceFs, wasmThemePath); exists {
			wasmSourcePath = wasmThemePath
		}

		if wasmSourcePath != "" {
			src, err := b.SourceFs.Open(wasmSourcePath)
			if err == nil {
				defer src.Close()
				_ = b.DestFs.MkdirAll(filepath.Dir(wasmDestPath), 0755)
				dest, err := b.DestFs.Create(wasmDestPath)
				if err == nil {
					defer dest.Close()
					if _, err := io.Copy(dest, src); err == nil {
						b.rnd.RegisterFile(wasmDestPath)
					}
				}
			}
		}

		// Ensure Site Logo is copied exactly (no WebP compression)
		if cfg.Logo != "" {
			if exists, _ := afero.Exists(b.SourceFs, cfg.Logo); exists {
				src, err := b.SourceFs.Open(cfg.Logo)
				if err == nil {
					defer src.Close()
					destPath := filepath.Join("public", cfg.Logo)
					_ = b.DestFs.MkdirAll(filepath.Dir(destPath), 0755)
					dest, err := b.DestFs.Create(destPath)
					if err == nil {
						defer dest.Close()
						if _, err := io.Copy(dest, src); err == nil {
							b.rnd.RegisterFile(destPath)
						}
					}
				}
			}
		}
	}()

	// 2. Esbuild Bundling (CSS/JS)
	go func() {
		defer wg.Done()
		assets, assetErr := utils.BuildAssetsEsbuild(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, b.rnd.RegisterFile, ".kosh-cache/assets")
		if assetErr != nil {
			b.logger.Error("Failed to build assets", "error", assetErr)
			return
		}
		b.rnd.SetAssets(assets)
	}()

	wg.Wait()
}
