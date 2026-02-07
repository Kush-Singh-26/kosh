package utils

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"
)

func BuildAssetsEsbuild(srcFs afero.Fs, destFs afero.Fs, srcDir, destDir string, minify bool, onWrite func(string)) (map[string]string, error) {
	fmt.Println("🎨 Building assets with Esbuild...")
	assets := make(map[string]string)

	var jsEntryPoints []string
	var cssEntryPoints []string

	// Find entry points
	err := afero.Walk(srcFs, srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".js":
			jsEntryPoints = append(jsEntryPoints, path)
		case ".css":
			cssEntryPoints = append(cssEntryPoints, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan for assets: %w", err)
	}

	process := func(entryPoints []string, bundle bool) error {
		if len(entryPoints) == 0 {
			return nil
		}
		buildOptions := api.BuildOptions{
			EntryPoints:       entryPoints,
			Bundle:            bundle,
			Write:             false,
			Outdir:            destDir,
			Outbase:           srcDir,
			MinifyWhitespace:  minify,
			MinifyIdentifiers: minify,
			MinifySyntax:      minify,
			Sourcemap:         api.SourceMapExternal,
			Metafile:          true,
			Loader: map[string]api.Loader{
				".woff2": api.LoaderFile,
				".woff":  api.LoaderFile,
				".ttf":   api.LoaderFile,
				".png":   api.LoaderFile,
				".webp":  api.LoaderFile,
				".svg":   api.LoaderFile,
			},
		}

		if minify {
			buildOptions.EntryNames = "[dir]/[name].[hash]"
			buildOptions.AssetNames = "assets/[name].[hash]"
		}

		result := api.Build(buildOptions)
		if len(result.Errors) > 0 {
			return fmt.Errorf("esbuild failed with %d errors", len(result.Errors))
		}

		for _, outFile := range result.OutputFiles {
			fullPath := filepath.ToSlash(outFile.Path)
			cwd, _ := os.Getwd()
			cwd = filepath.ToSlash(cwd)

			path := fullPath
			if strings.HasPrefix(fullPath, cwd) {
				path = strings.TrimPrefix(fullPath, cwd)
			}
			path = strings.TrimPrefix(path, "/")

			dir := filepath.Dir(path)
			if err := destFs.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := afero.WriteFile(destFs, path, outFile.Contents, 0644); err != nil {
				return err
			}
			if onWrite != nil {
				onWrite(path)
			}
		}

		// Use Metafile to map inputs to outputs correctly
		type Metafile struct {
			Outputs map[string]struct {
				EntryPoint string `json:"entryPoint"`
			} `json:"outputs"`
		}

		var meta Metafile
		if err := json.Unmarshal([]byte(result.Metafile), &meta); err != nil {
			return fmt.Errorf("failed to parse metafile: %w", err)
		}

		for outPath, outInfo := range meta.Outputs {
			if outInfo.EntryPoint == "" {
				continue
			}

			// Normalize paths for the assets map
			// EntryPoint might be "themes/blog/static/js/main.js"
			// We want the key to be "/static/js/main.js" for compatibility

			relEntryPoint := outInfo.EntryPoint
			relEntryPoint = strings.TrimPrefix(relEntryPoint, srcDir)
			if !strings.HasPrefix(relEntryPoint, "/") {
				relEntryPoint = "/" + relEntryPoint
			}
			// Ensure it starts with /static if it doesn't already
			// (Assuming srcDir content maps to /static in the output)
			key := "/static" + relEntryPoint
			if strings.HasPrefix(relEntryPoint, "/static") {
				key = relEntryPoint
			}

			val := outPath
			if strings.HasPrefix(val, "public/") {
				val = strings.TrimPrefix(val, "public")
			}
			if !strings.HasPrefix(val, "/") {
				val = "/" + val
			}

			assets[key] = val
		}
		return nil
	}

	// Process CSS with bundling (for @import and fonts)
	if err := process(cssEntryPoints, true); err != nil {
		return nil, err
	}

	// Process JS without bundling (to avoid wrapping standalone libraries)
	if err := process(jsEntryPoints, false); err != nil {
		return nil, err
	}

	return assets, nil
}
