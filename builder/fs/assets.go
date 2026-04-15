package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	urlPrefix       = "url("
	assetHashMinLen = 8
	assetHashMaxLen = 12
)

func restoreAssetsFromCache(opts restoreAssetsOptions) (map[string]string, bool, error) {
	if opts.cachePath == "" || opts.sink == nil || opts.destDir == "" {
		return nil, false, errors.New("restoreAssetsFromCache: missing required fields")
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

	eg, _ := errgroup.WithContext(context.Background())
	eg.SetLimit(runtime.NumCPU())

	for _, assetRelative := range assets {
		rel := assetRelative
		eg.Go(func() error {
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
	if err := eg.Wait(); err != nil {
		return nil, false, nil
	}

	return assets, true, nil
}

// BuildAssetsOptions configures esbuild asset processing.
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

type buildAssetsContext struct {
	srcFs            afero.Fs
	sink             ArtifactSink
	srcDir           string
	destDir          string
	minify           bool
	onWrite          func(string)
	cachePath        string
	sched            scheduler.BuildScheduler
	onAssetProcessed func()
}

func normalizeBuildAssetsOptions(opts BuildAssetsOptions) buildAssetsContext {
	return buildAssetsContext{
		srcFs:            opts.SrcFs,
		sink:             opts.Sink,
		srcDir:           NormalizePath(opts.SrcDir),
		destDir:          NormalizePath(opts.DestDir),
		minify:           opts.Minify,
		onWrite:          opts.OnWrite,
		cachePath:        opts.CacheDir,
		sched:            opts.Sched,
		onAssetProcessed: opts.OnAssetProcessed,
	}
}

func tryRestoreAssetCache(ctx buildAssetsContext, scan *assetScanResult, force bool) (map[string]string, bool, error) {
	if ctx.cachePath == "" || force {
		return nil, false, nil
	}
	cachePath := filepath.Join(ctx.cachePath, scan.hash)
	restored, ok, err := restoreAssetsFromCache(restoreAssetsOptions{
		cachePath:        cachePath,
		sink:             ctx.sink,
		destDir:          ctx.destDir,
		onWrite:          ctx.onWrite,
		onAssetProcessed: ctx.onAssetProcessed,
	})
	if err != nil {
		return nil, false, err
	}
	if ok {
		return restored, true, nil
	}
	return nil, false, nil
}

func buildEsbuildOptions(ctx buildAssetsContext, entryPoints []string, bundle bool) api.BuildOptions {
	buildOptions := api.BuildOptions{
		EntryPoints:       entryPoints,
		Bundle:            bundle,
		Write:             false,
		Outdir:            ctx.destDir,
		Outbase:           ctx.srcDir,
		MinifyWhitespace:  ctx.minify,
		MinifyIdentifiers: ctx.minify,
		MinifySyntax:      ctx.minify,
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

	if ctx.minify {
		buildOptions.EntryNames = "[dir]/[name].[hash]"
		buildOptions.AssetNames = "assets/[name].[hash]"
	}
	return buildOptions
}

func writeEsbuildOutputs(ctx buildAssetsContext, outFiles []api.OutputFile, cachePath string) error {
	eg, _ := errgroup.WithContext(context.Background())
	eg.SetLimit(runtime.NumCPU())

	for _, outFile := range outFiles {
		file := outFile
		eg.Go(func() error {
			if len(file.Contents) == 0 && !strings.HasSuffix(strings.ToLower(file.Path), ".map") {
				return fmt.Errorf("esbuild produced empty output for %s", file.Path)
			}
			fullPath := NormalizePath(file.Path)
			relPath, err := filepath.Rel(ctx.destDir, fullPath)
			if err != nil {
				return fmt.Errorf("failed to compute relative path for %s: %w", fullPath, err)
			}
			vfsPath := filepath.Join(ctx.destDir, relPath)
			vfsPath = normalizeEsbuildHashCase(vfsPath)

			contents := file.Contents
			ext := strings.ToLower(filepath.Ext(vfsPath))
			if ext == ".css" || ext == ".js" {
				contents = normalizeAssetURLHashes(contents)
			}

			dir := filepath.Dir(vfsPath)
			if err := ctx.sink.MkdirAll(dir); err != nil {
				return fmt.Errorf("failed to create directory for asset %s: %w", vfsPath, err)
			}

			if err := ctx.sink.WriteFile(vfsPath, contents); err != nil {
				return fmt.Errorf("failed to write asset %s: %w", vfsPath, err)
			}
			if ctx.onWrite != nil {
				ctx.onWrite(vfsPath)
			}
			if ctx.onAssetProcessed != nil {
				ctx.onAssetProcessed()
			}

			if cachePath != "" {
				rel, err := filepath.Rel(ctx.destDir, vfsPath)
				if err == nil {
					rel = normalizeEsbuildHashCase(rel)
					cacheFile := filepath.Join(cachePath, rel)
					_ = os.MkdirAll(filepath.Dir(cacheFile), defaultDirMode)
					_ = os.WriteFile(cacheFile, contents, defaultFileMode)
				}
			}
			return nil
		})
	}

	return eg.Wait()
}

func updateAssetsFromMetafile(metaJSON string, srcDir string, assets map[string]string, assetsMu *sync.Mutex) error {
	type metafileEntry struct {
		EntryPoint string `json:"entryPoint"`
	}
	type metafile struct {
		Outputs map[string]metafileEntry `json:"outputs"`
	}

	var meta metafile
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
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

func buildEntryPoints(ctx buildAssetsContext, entryPoints []string, bundle bool, assets map[string]string, assetsMu *sync.Mutex, cachePath string) error {
	if len(entryPoints) == 0 {
		return nil
	}
	buildOptions := buildEsbuildOptions(ctx, entryPoints, bundle)
	result := api.Build(buildOptions)
	if len(result.Errors) > 0 {
		for _, errEntry := range result.Errors {
			slog.Error("esbuild error", "message", errEntry.Text)
		}
		return fmt.Errorf("esbuild failed with %d errors", len(result.Errors))
	}

	if err := writeEsbuildOutputs(ctx, result.OutputFiles, cachePath); err != nil {
		return err
	}

	return updateAssetsFromMetafile(result.Metafile, ctx.srcDir, assets, assetsMu)
}

// BuildAssetsEsbuild builds CSS/JS assets with esbuild and returns the asset map.
func BuildAssetsEsbuild(opts BuildAssetsOptions) (map[string]string, error) {
	ctx := normalizeBuildAssetsOptions(opts)
	scan, err := scanAssets(ctx.srcFs, ctx.srcDir)
	if err != nil {
		return nil, err
	}

	if restored, ok, err := tryRestoreAssetCache(ctx, scan, opts.Force); err != nil {
		return nil, err
	} else if ok {
		return restored, nil
	}

	if ctx.sched != nil {
		if err := ctx.sched.Acquire(context.Background(), scheduler.TaskAsset); err != nil {
			return nil, err
		}
		defer ctx.sched.Release(scheduler.TaskAsset)
	}

	assets := make(map[string]string)
	var assetsMu sync.Mutex

	cachePath := ""
	if ctx.cachePath != "" && !opts.Force {
		cachePath = filepath.Join(ctx.cachePath, scan.hash)
	}

	buildGroup, _ := errgroup.WithContext(context.Background())

	if scan.hasCSS() {
		buildGroup.Go(func() error {
			return buildEntryPoints(ctx, scan.points.css, true, assets, &assetsMu, cachePath)
		})
	}
	if scan.hasJS() {
		buildGroup.Go(func() error {
			return buildEntryPoints(ctx, scan.points.js, true, assets, &assetsMu, cachePath)
		})
	}

	if err := buildGroup.Wait(); err != nil {
		return nil, err
	}

	if cachePath != "" {
		mapData, _ := json.Marshal(assets)
		_ = os.WriteFile(filepath.Join(cachePath, "map.json"), mapData, defaultFileMode)
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
	if len(s) < assetHashMinLen || len(s) > assetHashMaxLen {
		return false
	}
	for _, char := range s {
		if (char < '0' || char > '9') && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}
	return true
}

// normalizeAssetURLHashes lowercases hash segments inside url(...) references in CSS/JS.
// Uses byte scanning instead of regex for O(n) performance.
func normalizeAssetURLHashes(content []byte) []byte {
	if !bytes.Contains(content, []byte(urlPrefix)) {
		return content
	}

	result := make([]byte, 0, len(content))
	i := 0
	for i < len(content) {
		idx := bytes.Index(content[i:], []byte(urlPrefix))
		if idx < 0 {
			result = append(result, content[i:]...)
			break
		}
		// Copy everything before url(
		result = append(result, content[i:i+idx]...)
		urlStart := i + idx // position of 'u' in url(

		// Skip "url("
		innerStart := urlStart + len(urlPrefix)
		// Skip optional whitespace
		for innerStart < len(content) && isWhitespace(content[innerStart]) {
			innerStart++
		}

		if innerStart < len(content) && (content[innerStart] == '"' || content[innerStart] == '\'') {
			i = parseQuotedURL(content, urlStart, innerStart, &result)
		} else {
			i = parseUnquotedURL(content, urlStart, innerStart, &result)
		}
	}
	return result
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func parseQuotedURL(content []byte, urlStart, innerStart int, result *[]byte) int {
	quote := content[innerStart]
	urlEnd := bytes.IndexByte(content[innerStart+1:], quote)
	if urlEnd < 0 {
		// No closing quote, copy rest as-is
		*result = append(*result, content[urlStart:]...)
		return len(content)
	}
	urlEnd += innerStart + 1
	urlInner := content[innerStart+1 : urlEnd]
	normalized := normalizeURLHash(urlInner)

	// Reconstruct url("...") or url('...')
	*result = append(*result, []byte(urlPrefix)...)
	*result = append(*result, quote)
	*result = append(*result, normalized...)
	*result = append(*result, quote)
	*result = append(*result, ')')

	// Advance past the closing ')' if present to avoid duplicating it
	i := urlEnd + 1
	parenIdx := i
	for parenIdx < len(content) && isWhitespace(content[parenIdx]) {
		parenIdx++
	}
	if parenIdx < len(content) && content[parenIdx] == ')' {
		i = parenIdx + 1
	}
	return i
}

func parseUnquotedURL(content []byte, urlStart, innerStart int, result *[]byte) int {
	// Unquoted: find closing ')'
	urlEnd := bytes.IndexByte(content[innerStart:], ')')
	if urlEnd < 0 {
		*result = append(*result, content[urlStart:]...)
		return len(content)
	}
	urlEnd += innerStart
	// Trim trailing whitespace
	inner := bytes.TrimSpace(content[innerStart:urlEnd])
	normalized := normalizeURLHash(inner)

	*result = append(*result, []byte(urlPrefix)...)
	*result = append(*result, normalized...)
	*result = append(*result, ')')
	return urlEnd + 1
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
