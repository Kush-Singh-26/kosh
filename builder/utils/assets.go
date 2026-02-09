package utils

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"
)

func BuildAssetsEsbuild(srcFs afero.Fs, destFs afero.Fs, srcDir, destDir string, minify bool, onWrite func(string), cacheDir string) (map[string]string, error) {
	assets := make(map[string]string)

	var jsEntryPoints []string
	var cssEntryPoints []string

	// Calculate input hash
	inputHash := md5.New()

	// Find entry points
	err := afero.Walk(srcFs, srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".js":
			jsEntryPoints = append(jsEntryPoints, path)
		case ".css":
			cssEntryPoints = append(cssEntryPoints, path)
		}

		// Add to hash (path + mtime + size)
		fmt.Fprintf(inputHash, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan for assets: %w", err)
	}

	currentHash := hex.EncodeToString(inputHash.Sum(nil))
	cachePath := ""
	if cacheDir != "" {
		cachePath = filepath.Join(cacheDir, currentHash)
		// Check cache
		if info, err := os.Stat(cachePath); err == nil && info.IsDir() {
			// Restore from cache
			mapFile := filepath.Join(cachePath, "map.json")
			if mapData, err := os.ReadFile(mapFile); err == nil {
				if err := json.Unmarshal(mapData, &assets); err == nil {
					// Restore files
					err = filepath.Walk(cachePath, func(path string, info fs.FileInfo, err error) error {
						if info.IsDir() || filepath.Base(path) == "map.json" {
							return nil
						}
						relPath, _ := filepath.Rel(cachePath, path)
						// destDir/relPath
						// But relPath in cache is flattened?
						// Wait, esbuild output preserves structure if Outbase is used.
						// We need to mirror structure.

						// Let's assume cache structure matches public/static structure
						// Read file
						data, err := os.ReadFile(path)
						if err != nil {
							return err
						}

						// Write to destFs
						// destDir is public/static
						// relPath is css/main.css
						destPath := filepath.Join(destDir, relPath)
						if err := destFs.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
							return err
						}
						if err := afero.WriteFile(destFs, destPath, data, 0644); err != nil {
							return err
						}
						if onWrite != nil {
							onWrite(destPath)
						}
						return nil
					})
					if err == nil {
						return assets, nil // Cache Hit!
					}
				}
			}
		}
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

			// Cache the output file
			if cachePath != "" {
				// Relativize path from destDir (public/static)
				// path is public/static/css/main.css
				// rel is css/main.css
				rel, err := filepath.Rel(destDir, path)
				if err == nil {
					cacheFile := filepath.Join(cachePath, rel)
					_ = os.MkdirAll(filepath.Dir(cacheFile), 0755)
					_ = os.WriteFile(cacheFile, outFile.Contents, 0644)
				}
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

	// Save map to cache
	if cachePath != "" {
		mapData, _ := json.Marshal(assets)
		_ = os.WriteFile(filepath.Join(cachePath, "map.json"), mapData, 0644)
	}

	return assets, nil
}
