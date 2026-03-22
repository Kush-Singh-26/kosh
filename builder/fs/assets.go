package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

func restoreAssetsFromCache(cachePath string, sink ArtifactSink, destDir string, onWrite func(string)) (map[string]string, bool, error) {
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		return nil, false, nil
	}

	mapFile := filepath.Join(cachePath, "map.json")
	mapData, err := os.ReadFile(mapFile)
	if err != nil || json.Unmarshal(mapData, new(map[string]string)) != nil {
		return nil, false, nil
	}

	var assets map[string]string
	if err := json.Unmarshal(mapData, &assets); err != nil {
		return nil, false, nil
	}

	err = filepath.WalkDir(cachePath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || filepath.Base(path) == "map.json" {
			return walkErr
		}
		path = NormalizePath(path)
		relPath, _ := SafeRel(cachePath, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
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
	if err != nil {
		return nil, false, nil
	}

	return assets, true, nil
}

func BuildAssetsEsbuild(srcFs afero.Fs, sink ArtifactSink, srcDir, destDir string, minify bool, onWrite func(string), cacheDir string, force bool, sched scheduler.BuildScheduler) (map[string]string, error) {
	srcDir = NormalizePath(srcDir)
	destDir = NormalizePath(destDir)

	scan, err := scanAssets(srcFs, srcDir)
	if err != nil {
		return nil, err
	}

	assets := make(map[string]string)
	cachePath := ""
	if cacheDir != "" && !force {
		cachePath = filepath.Join(cacheDir, scan.hash)
		if restored, ok, _ := restoreAssetsFromCache(cachePath, sink, destDir, onWrite); ok {
			return restored, nil
		}
	}

	if sched != nil {
		if err := sched.Acquire(context.Background(), scheduler.TaskAsset); err != nil {
			return nil, err
		}
		defer sched.Release(scheduler.TaskAsset)
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
			relPath, err := filepath.Rel(destDir, fullPath)
			if err != nil {
				return fmt.Errorf("failed to compute relative path for %s: %w", fullPath, err)
			}
			vfsPath := filepath.Join(destDir, relPath)
			vfsPath = normalizeEsbuildHashCase(vfsPath)

			contents := outFile.Contents
			ext := strings.ToLower(filepath.Ext(vfsPath))
			if ext == ".css" || ext == ".js" {
				contents = normalizeAssetURLHashes(contents)
			}

			if err := sink.MkdirAll(filepath.Dir(vfsPath)); err != nil {
				return err
			}
			if err := sink.WriteFile(vfsPath, contents); err != nil {
				return err
			}
			if onWrite != nil {
				onWrite(vfsPath)
			}

			if cachePath != "" {
				rel, err := filepath.Rel(destDir, vfsPath)
				if err == nil {
					rel = normalizeEsbuildHashCase(rel)
					cacheFile := filepath.Join(cachePath, rel)
					_ = os.MkdirAll(filepath.Dir(cacheFile), 0755)
					_ = os.WriteFile(cacheFile, contents, 0644)
				}
			}
		}

		type metafileEntry struct {
			EntryPoint string `json:"entryPoint"`
		}
		type metafile struct {
			Outputs map[string]metafileEntry `json:"outputs"`
		}

		var meta metafile
		if err := json.Unmarshal([]byte(result.Metafile), &meta); err != nil {
			return fmt.Errorf("failed to parse metafile: %w", err)
		}

		for outPath, outInfo := range meta.Outputs {
			if outInfo.EntryPoint == "" {
				continue
			}

			entryPointAbs, _ := filepath.Abs(outInfo.EntryPoint)
			relEntryPoint, _ := SafeRel(srcDir, NormalizePath(entryPointAbs))
			relEntryPoint = strings.TrimPrefix(filepath.ToSlash(relEntryPoint), "/")

			key := "/static/" + relEntryPoint

			val := filepath.ToSlash(outPath)
			if idx := strings.Index(val, "/static/"); idx != -1 {
				val = val[idx:]
			} else if !strings.HasPrefix(val, "/") {
				val = "/" + val
			}

			val = normalizeEsbuildHashCase(val)

			assetsMu.Lock()
			assets[key] = val
			assetsMu.Unlock()
		}
		return nil
	}

	buildGroup, _ := errgroup.WithContext(context.Background())

	if scan.hasCSS() {
		buildGroup.Go(func() error {
			return process(scan.points.css, true)
		})
	}
	if scan.hasJS() {
		buildGroup.Go(func() error {
			return process(scan.points.js, true)
		})
	}

	if err := buildGroup.Wait(); err != nil {
		return nil, err
	}

	if cachePath != "" {
		mapData, _ := json.Marshal(assets)
		_ = os.WriteFile(filepath.Join(cachePath, "map.json"), mapData, 0644)
	}

	return assets, nil
}

func normalizeEsbuildHashCase(path string) string {
	if path == "" {
		return path
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	segments := strings.Split(base, ".")
	if len(segments) < 2 {
		return path
	}

	changed := false
	for i, seg := range segments {
		if isAlphanumericHash(seg) {
			lowered := strings.ToLower(seg)
			if lowered != seg {
				segments[i] = lowered
				changed = true
			}
		}
	}

	if !changed {
		return path
	}

	return filepath.ToSlash(filepath.Join(dir, strings.Join(segments, ".")))
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

func normalizeAssetURLHashes(content []byte) []byte {
	// Find url(...) references with hash segments and lowercase them
	re := regexp.MustCompile(`url\(["']?([^"')]+)["']?\)`)
	return re.ReplaceAllFunc(content, func(match []byte) []byte {
		urlRe := regexp.MustCompile(`url\(["']?([^"')]+)["']?\)`)
		submatches := urlRe.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		urlPath := string(submatches[1])
		dir := path.Dir(urlPath) // use forward-slash path for URLs
		base := path.Base(urlPath)
		segments := strings.Split(base, ".")
		changed := false
		for i, seg := range segments {
			if isAlphanumericHash(seg) {
				lowered := strings.ToLower(seg)
				if lowered != seg {
					segments[i] = lowered
					changed = true
				}
			}
		}
		if !changed {
			return match
		}
		newBase := strings.Join(segments, ".")
		var newPath string
		if dir == "." {
			newPath = newBase
		} else {
			newPath = dir + "/" + newBase
		}
		// Try to preserve original quoting style
		if strings.Contains(string(match), "\"") {
			return []byte(fmt.Sprintf("url(\"%s\")", newPath))
		} else if strings.Contains(string(match), "'") {
			return []byte(fmt.Sprintf("url('%s')", newPath))
		}
		return []byte(fmt.Sprintf("url(%s)", newPath))
	})
}
