package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

type restoreAssetsOptions struct {
	cachePath        string
	sink             ArtifactSink
	destDir          string
	onWrite          func(string)
	onAssetProcessed func()
}

func restoreAssetsFromCache(opts restoreAssetsOptions) (map[string]string, bool, error) {
	if opts.cachePath == "" || opts.sink == nil || opts.destDir == "" {
		return nil, false, fmt.Errorf("restoreAssetsFromCache: missing required fields")
	}
	if info, err := os.Stat(opts.cachePath); err != nil || !info.IsDir() {
		return nil, false, nil
	}

	mapFile := filepath.Join(opts.cachePath, "map.json")
	mapData, err := os.ReadFile(mapFile)
	if err != nil {
		return nil, false, nil
	}
	var assets map[string]string
	if err := json.Unmarshal(mapData, &assets); err != nil {
		return nil, false, nil
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())

	for _, v := range assets {
		rel := v
		g.Go(func() error {
			cacheFile := filepath.Join(opts.cachePath, rel)
			destPath := filepath.Join(opts.destDir, rel)
			data, err := os.ReadFile(cacheFile)
			if err != nil {
				return err
			}
			if err := opts.sink.MkdirAll(filepath.Dir(destPath)); err != nil {
				return err
			}
			if err := opts.sink.WriteFile(destPath, data); err != nil {
				return err
			}
			if opts.onWrite != nil {
				opts.onWrite(destPath)
			}
			if opts.onAssetProcessed != nil {
				opts.onAssetProcessed()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, false, nil
	}

	return assets, true, nil
}

type BuildAssetsOptions struct {
	SrcFs            afero.Fs
	Sink             ArtifactSink
	SrcDir           string
	DestDir          string
	Minify           bool
	OnWrite          func(string)
	CacheDir         string
	Force            bool
	Sched            scheduler.BuildScheduler
	OnAssetProcessed func()
}

func BuildAssetsEsbuild(opts BuildAssetsOptions) (map[string]string, error) {
	srcFs := opts.SrcFs
	sink := opts.Sink
	srcDir := opts.SrcDir
	destDir := opts.DestDir
	minify := opts.Minify
	onWrite := opts.OnWrite
	cacheDir := opts.CacheDir
	force := opts.Force
	sched := opts.Sched
	onAssetProcessed := opts.OnAssetProcessed

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
		restored, ok, err := restoreAssetsFromCache(restoreAssetsOptions{
			cachePath:        cachePath,
			sink:             sink,
			destDir:          destDir,
			onWrite:          onWrite,
			onAssetProcessed: onAssetProcessed,
		})
		if err != nil {
			return nil, err
		}
		if ok {
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

		g, _ := errgroup.WithContext(context.Background())
		g.SetLimit(runtime.NumCPU())

		for _, outFile := range result.OutputFiles {
			file := outFile
			g.Go(func() error {
				if len(file.Contents) == 0 && !strings.HasSuffix(strings.ToLower(file.Path), ".map") {
					return fmt.Errorf("esbuild produced empty output for %s", file.Path)
				}
				fullPath := NormalizePath(file.Path)
				relPath, err := filepath.Rel(destDir, fullPath)
				if err != nil {
					return fmt.Errorf("failed to compute relative path for %s: %w", fullPath, err)
				}
				vfsPath := filepath.Join(destDir, relPath)
				vfsPath = normalizeEsbuildHashCase(vfsPath)

				contents := file.Contents
				ext := strings.ToLower(filepath.Ext(vfsPath))
				if ext == ".css" || ext == ".js" {
					contents = normalizeAssetURLHashes(contents)
				}

				dir := filepath.Dir(vfsPath)
				if err := sink.MkdirAll(dir); err != nil {
					return fmt.Errorf("failed to create directory for asset %s: %w", vfsPath, err)
				}

				if err := sink.WriteFile(vfsPath, contents); err != nil {
					return fmt.Errorf("failed to write asset %s: %w", vfsPath, err)
				}
				if onWrite != nil {
					onWrite(vfsPath)
				}
				if onAssetProcessed != nil {
					onAssetProcessed()
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
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
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

// normalizeAssetURLHashes lowercases hash segments inside url(...) references in CSS/JS.
// Uses byte scanning instead of regex for O(n) performance.
func normalizeAssetURLHashes(content []byte) []byte {
	if !bytes.Contains(content, []byte("url(")) {
		return content
	}

	result := make([]byte, 0, len(content))
	i := 0
	for i < len(content) {
		idx := bytes.Index(content[i:], []byte("url("))
		if idx < 0 {
			result = append(result, content[i:]...)
			break
		}
		// Copy everything before url(
		result = append(result, content[i:i+idx]...)
		urlStart := i + idx // position of 'u' in url(

		// Skip "url("
		innerStart := urlStart + 4
		// Skip optional whitespace
		for innerStart < len(content) && (content[innerStart] == ' ' || content[innerStart] == '\t' || content[innerStart] == '\n' || content[innerStart] == '\r') {
			innerStart++
		}

		var quote byte
		var urlEnd int
		if innerStart < len(content) && (content[innerStart] == '"' || content[innerStart] == '\'') {
			quote = content[innerStart]
			urlEnd = bytes.IndexByte(content[innerStart+1:], quote)
			if urlEnd < 0 {
				// No closing quote, copy rest as-is
				result = append(result, content[urlStart:]...)
				i = len(content)
				continue
			}
			urlEnd += innerStart + 1
			urlInner := content[innerStart+1 : urlEnd]
			normalized := normalizeURLHash(urlInner)

			// Reconstruct url("...") or url('...')
			result = append(result, []byte("url(")...)
			result = append(result, quote)
			result = append(result, normalized...)
			result = append(result, quote)
			result = append(result, ')')
			// Advance past the closing ')' if present to avoid duplicating it
			i = urlEnd + 1
			parenIdx := i
			for parenIdx < len(content) && (content[parenIdx] == ' ' || content[parenIdx] == '\t' || content[parenIdx] == '\n' || content[parenIdx] == '\r') {
				parenIdx++
			}
			if parenIdx < len(content) && content[parenIdx] == ')' {
				i = parenIdx + 1
			}
		} else {
			// Unquoted: find closing ')'
			urlEnd = bytes.IndexByte(content[innerStart:], ')')
			if urlEnd < 0 {
				result = append(result, content[urlStart:]...)
				i = len(content)
				continue
			}
			urlEnd += innerStart
			// Trim trailing whitespace
			inner := bytes.TrimSpace(content[innerStart:urlEnd])
			normalized := normalizeURLHash(inner)

			result = append(result, []byte("url(")...)
			result = append(result, normalized...)
			result = append(result, ')')
			i = urlEnd + 1
		}
	}
	return result
}

// normalizeURLHash lowercases hash segments in a URL path like "/static/css/layout.ABC123.css"
func normalizeURLHash(url []byte) []byte {
	urlStr := string(url)
	dotIdx := strings.LastIndex(urlStr, ".")
	if dotIdx < 0 {
		return url
	}
	base := path.Base(urlStr)
	dir := path.Dir(urlStr)

	segments := strings.Split(base, ".")
	changed := false
	for j, seg := range segments {
		if isAlphanumericHash(seg) {
			lowered := strings.ToLower(seg)
			if lowered != seg {
				segments[j] = lowered
				changed = true
			}
		}
	}
	if !changed {
		return url
	}
	newBase := strings.Join(segments, ".")
	if dir == "." {
		return []byte(newBase)
	}
	return []byte(dir + "/" + newBase)
}
