package run

import (
	"fmt"
	"io"
	"my-ssg/builder/utils"
	"sync"

	"github.com/spf13/afero"
)

func (b *Builder) copyStaticAndBuildAssets() {
	cfg := b.cfg
	var wg sync.WaitGroup
	wg.Add(2)

	// 1. Static Copy (excluding source CSS/JS handled by esbuild)
	go func() {
		defer wg.Done()
		if exists, _ := afero.Exists(b.SourceFs, cfg.StaticDir); exists {
			// Exclude .css and .js files from raw copy (they're handled by esbuild)
			if err := utils.CopyDirVFS(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, []string{".css", ".js"}, b.rnd.RegisterFile); err != nil {
				fmt.Printf("⚠️ Failed to copy static assets: %v\n", err)
			}
		}

		// Copy wasm_exec.js separately (it's needed by the WASM search but shouldn't be processed by esbuild)
		wasmExecPath := cfg.StaticDir + "/js/wasm_exec.js"
		if exists, _ := afero.Exists(b.SourceFs, wasmExecPath); exists {
			src, err := b.SourceFs.Open(wasmExecPath)
			if err == nil {
				defer src.Close()
				dest, err := b.DestFs.Create("public/static/js/wasm_exec.js")
				if err == nil {
					defer dest.Close()
					if _, err := io.Copy(dest, src); err == nil {
						b.rnd.RegisterFile("public/static/js/wasm_exec.js")
					}
				}
			}
		}
	}()

	// 2. Esbuild Bundling (CSS/JS)
	go func() {
		defer wg.Done()
		assets, assetErr := utils.BuildAssetsEsbuild(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, b.rnd.RegisterFile)
		if assetErr == nil {
			b.rnd.SetAssets(assets)
		}
	}()

	wg.Wait()
}
