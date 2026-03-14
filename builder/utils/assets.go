package utils

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/errgroup"
)

func BuildAssetsEsbuild(srcFs afero.Fs, sink ArtifactSink, srcDir, destDir string, minify bool, onWrite func(string), cacheDir string, force bool) (map[string]string, error) {
	// slog.Info("Building assets", "srcDir", srcDir, "destDir", destDir)
	srcDir = NormalizePath(srcDir)
	destDir = NormalizePath(destDir)
	assets := make(map[string]string)

	var jsEntryPoints []string
	var cssEntryPoints []string
	var walkMu sync.Mutex

	// Calculate input hash
	type fileMeta struct {
		path  string
		size  int64
		mtime int64
	}
	var metas []fileMeta

	// Find entry points
	err := ParallelWalk(context.Background(), srcFs, filepath.FromSlash(srcDir), 0, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		baseName := filepath.Base(path)

		// Skip files that must be copied directly without esbuild processing
		if baseName == "wasm_engine.js" || baseName == "engine.js" || baseName == "force-graph.js" {
			return nil
		}

		walkMu.Lock()
		defer walkMu.Unlock()

		switch ext {
		case ".js":
			jsEntryPoints = append(jsEntryPoints, path)
		case ".css":
			cssEntryPoints = append(cssEntryPoints, path)
		}

		// Add to metadata slice instead of hash directly
		metas = append(metas, fileMeta{
			path:  path,
			size:  info.Size(),
			mtime: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan for assets: %w", err)
	}

	// Sort metadata for deterministic hashing
	slices.SortFunc(metas, func(a, b fileMeta) int {
		return strings.Compare(a.path, b.path)
	})

	inputHash := xxh3.New()
	for _, m := range metas {
		if _, err := fmt.Fprintf(inputHash, "%s:%d:%d;", m.path, m.size, m.mtime); err != nil {
			return nil, fmt.Errorf("failed to write to input hash: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan for assets: %w", err)
	}

	sum := inputHash.Sum128()
	b := sum.Bytes()
	currentHash := hex.EncodeToString(b[:])
	cachePath := ""
	if cacheDir != "" {
		cachePath = filepath.Join(cacheDir, currentHash)
		// Check cache (skip if force is true)
		if !force {
			if info, err := os.Stat(cachePath); err == nil && info.IsDir() {
				// Restore from cache
				mapFile := filepath.Join(cachePath, "map.json")
				if mapData, err := os.ReadFile(mapFile); err == nil {
					if err := json.Unmarshal(mapData, &assets); err == nil {
						// Restore files
						err = filepath.WalkDir(cachePath, func(path string, d fs.DirEntry, walkErr error) error {
							if d.IsDir() || filepath.Base(path) == "map.json" {
								return nil
							}
							path = NormalizePath(path)
							relPath, _ := SafeRel(cachePath, path)
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

							// Write to sink
							// destDir is public/static
							// relPath is css/main.css
							destPath := filepath.Join(destDir, relPath)
							if err := sink.MkdirAll(filepath.Dir(destPath)); err != nil {
								return err
							}
							if err := sink.WriteFile(destPath, data); err != nil {
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
	}

	var assetsMu sync.Mutex

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
			for _, e := range result.Errors {
				slog.Error("esbuild error", "message", e.Text)
			}
			return fmt.Errorf("esbuild failed with %d errors", len(result.Errors))
		}

		for _, outFile := range result.OutputFiles {
			if len(outFile.Contents) == 0 && !strings.HasSuffix(strings.ToLower(outFile.Path), ".map") {
				return fmt.Errorf("esbuild produced empty output for %s", outFile.Path)
			}
			fullPath := NormalizePath(outFile.Path)
			// Compute relative path from destDir for VFS
			relPath, err := filepath.Rel(destDir, fullPath)
			if err != nil {
				return fmt.Errorf("failed to compute relative path for %s: %w", fullPath, err)
			}
			vfsPath := filepath.Join(destDir, relPath)

			dir := filepath.Dir(vfsPath)
			if err := sink.MkdirAll(dir); err != nil {
				return err
			}
			if err := sink.WriteFile(vfsPath, outFile.Contents); err != nil {
				return err
			}
			if onWrite != nil {
				onWrite(vfsPath)
			}

			// Cache the output file
			if cachePath != "" {
				// Relativize path from destDir (public/static)
				// vfsPath is public/static/css/main.css
				// rel is css/main.css
				rel, err := filepath.Rel(destDir, vfsPath)
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
			// EntryPoint might be "themes/<theme>/static/js/main.js"
			// We want the key to be "/static/js/main.js" for compatibility

			entryPointAbs, _ := filepath.Abs(outInfo.EntryPoint)
			relEntryPoint, _ := SafeRel(srcDir, NormalizePath(entryPointAbs))
			relEntryPoint = strings.TrimPrefix(filepath.ToSlash(relEntryPoint), "/")

			key := "/static/" + relEntryPoint

			val := filepath.ToSlash(outPath)
			// Find /static/ in the path to handle any output directory
			if idx := strings.Index(val, "/static/"); idx != -1 {
				val = val[idx:]
			} else if !strings.HasPrefix(val, "/") {
				val = "/" + val
			}

			// Normalize hash portion to lowercase for case-insensitive filesystems (Windows)
			val = normalizeHashCase(val)

			assetsMu.Lock()
			assets[key] = val
			assetsMu.Unlock()
		}
		return nil
	}

	// Process CSS and JS concurrently — esbuild is internally thread-safe
	// and the two builds operate on different file types with no overlap.
	buildGroup, _ := errgroup.WithContext(context.Background())

	if len(cssEntryPoints) > 0 {
		buildGroup.Go(func() error {
			return process(cssEntryPoints, true)
		})
	}
	if len(jsEntryPoints) > 0 {
		buildGroup.Go(func() error {
			return process(jsEntryPoints, true)
		})
	}

	if err := buildGroup.Wait(); err != nil {
		return nil, err
	}

	// Save map to cache
	if cachePath != "" {
		mapData, _ := json.Marshal(assets)
		_ = os.WriteFile(filepath.Join(cachePath, "map.json"), mapData, 0644)
	}

	return assets, nil
}

func normalizeHashCase(path string) string {
	// Find pattern like .HASH12345. where hash is alphanumeric and 8-12 chars
	for i := strings.Index(path, "."); i >= 0 && i < len(path)-10; i++ {
		// Look for extension-like pattern after a dot
		if i+9 < len(path) {
			hashPart := path[i+1 : i+9]
			if isAlphanumericHash(hashPart) {
				return path[:i+1] + strings.ToLower(hashPart) + path[i+9:]
			}
		}
	}
	return path
}

func isAlphanumericHash(s string) bool {
	if len(s) < 8 || len(s) > 12 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
