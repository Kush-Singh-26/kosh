package run

import (
	"fmt"
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
			// Exclude .css and .js files from raw copy
			if err := utils.CopyDirVFS(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, []string{".css", ".js"}, b.rnd.RegisterFile); err != nil {
				fmt.Printf("⚠️ Failed to copy static assets: %v\n", err)
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
