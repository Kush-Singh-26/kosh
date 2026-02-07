package run

import (
	"fmt"
	"github.com/spf13/afero"
	"my-ssg/builder/utils"
)

func (b *Builder) copyStaticAndBuildAssets() {
	cfg := b.cfg
	if exists, _ := afero.Exists(b.SourceFs, cfg.StaticDir); exists {
		if err := utils.CopyDirVFS(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, b.rnd.RegisterFile); err != nil {
			fmt.Printf("⚠️ Failed to copy static assets: %v\n", err)
		}
	}
	assets, assetErr := utils.BuildAssetsEsbuild(b.SourceFs, b.DestFs, cfg.StaticDir, "public/static", cfg.CompressImages, b.rnd.RegisterFile)
	if assetErr == nil {
		b.rnd.SetAssets(assets)
	}
}
