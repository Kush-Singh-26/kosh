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
	"slices"
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
	minify           bool
}

const (
	urlPrefix       = "url("
	assetHashMinLen = 8
	assetHashMaxLen = 12
)

// Asset URLs remain in map.json; this manifest records every output relative to DestDir.
// Caches without a manifest cannot prove secondary outputs are complete and are rebuilt.
type assetCacheManifest struct {
	Version int      `json:"version"`
	Minify  bool     `json:"minify"`
	Outputs []string `json:"outputs"`
}

func validAssetOutputPath(rel string) bool {
	return rel != "." && filepath.IsLocal(rel) && path.Clean(rel) == rel &&
		!strings.ContainsAny(rel, "\\:\x00")
}

func readAssetCache(cachePath string, minify bool) (map[string]string, map[string][]byte, error) {
	root, err := os.OpenRoot(cachePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()

	manifestData, err := root.ReadFile("outputs.json")
	if err != nil {
		return nil, nil, err
	}
	var manifest assetCacheManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, err
	}
	if manifest.Version != 1 || manifest.Minify != minify || manifest.Outputs == nil {
		return nil, nil, errors.New("incompatible asset cache manifest")
	}
	files := make(map[string][]byte, len(manifest.Outputs))
	for _, rel := range manifest.Outputs {
		if !validAssetOutputPath(rel) {
			return nil, nil, fmt.Errorf("invalid cached asset path: %q", rel)
		}
		if minify && strings.HasSuffix(strings.ToLower(rel), ".map") {
			continue
		}
		data, err := root.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return nil, nil, err
		}
		if len(data) == 0 && !strings.HasSuffix(strings.ToLower(rel), ".map") {
			return nil, nil, fmt.Errorf("empty cached asset: %q", rel)
		}
		files[rel] = data
	}
	assets, err := readCachedAssetMap(root, files)
	return assets, files, err
}

func readCachedAssetMap(root *os.Root, files map[string][]byte) (map[string]string, error) {
	mapData, err := root.ReadFile("map.json")
	if err != nil {
		return nil, err
	}
	var assets map[string]string
	if err := json.Unmarshal(mapData, &assets); err != nil {
		return nil, err
	}
	if assets == nil {
		return nil, errors.New("missing asset map")
	}
	for _, url := range assets {
		rel, ok := strings.CutPrefix(url, "/static/")
		if !ok || !validAssetOutputPath(rel) {
			return nil, fmt.Errorf("invalid cached asset URL: %q", url)
		}
		if _, ok := files[rel]; !ok {
			return nil, fmt.Errorf("missing cached entrypoint: %q", rel)
		}
	}
	return assets, nil
}

func restoreAssetsFromCache(opts restoreAssetsOptions) (map[string]string, bool, error) {
	if opts.cachePath == "" || opts.sink == nil || opts.destDir == "" {
		return nil, false, errors.New("restoreAssetsFromCache: missing required fields")
	}
	assets, files, err := readAssetCache(opts.cachePath, opts.minify)
	if err != nil {
		return nil, false, nil
	}

	eg, _ := errgroup.WithContext(context.Background())
	eg.SetLimit(runtime.NumCPU())

	for rel, data := range files {
		eg.Go(func() error {
			destPath := filepath.Join(opts.destDir, filepath.FromSlash(rel))
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
		minify:           ctx.minify,
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
	// Production (minify on) ships no sourcemaps; they'd double the CSS/JS payload
	// in output. Opt out of minification to keep them for debugging.
	sourcemap := api.SourceMapExternal
	if ctx.minify {
		sourcemap = api.SourceMapNone
	}

	buildOptions := api.BuildOptions{
		EntryPoints:       entryPoints,
		Bundle:            bundle,
		Write:             false,
		Format:            api.FormatESModule,
		Splitting:         bundle,
		Outdir:            ctx.destDir,
		Outbase:           ctx.srcDir,
		MinifyWhitespace:  ctx.minify,
		MinifyIdentifiers: ctx.minify,
		MinifySyntax:      ctx.minify,
		Sourcemap:         sourcemap,
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

func esbuildOutputPaths(destDir string, outFiles []api.OutputFile) ([]string, error) {
	outputs := make([]string, 0, len(outFiles))
	for _, file := range outFiles {
		rel, err := filepath.Rel(destDir, NormalizePath(file.Path))
		if err != nil {
			return nil, err
		}
		rel = normalizeEsbuildHashCase(filepath.ToSlash(rel))
		if !validAssetOutputPath(rel) {
			return nil, fmt.Errorf("esbuild output escapes destination: %q", file.Path)
		}
		outputs = append(outputs, rel)
	}
	return outputs, nil
}

func openAssetCacheRoot(cachePath string) *os.Root {
	if cachePath == "" {
		return nil
	}
	if err := os.MkdirAll(cachePath, defaultDirMode); err != nil {
		return nil
	}
	root, _ := os.OpenRoot(cachePath)
	return root
}

func writeEsbuildOutputs(ctx buildAssetsContext, outFiles []api.OutputFile, cachePath string) error {
	outputs, err := esbuildOutputPaths(ctx.destDir, outFiles)
	if err != nil {
		return err
	}
	cacheRoot := openAssetCacheRoot(cachePath)
	if cacheRoot != nil {
		defer func() { _ = cacheRoot.Close() }()
	}
	eg, _ := errgroup.WithContext(context.Background())
	eg.SetLimit(runtime.NumCPU())

	for i, file := range outFiles {
		eg.Go(func() error {
			if len(file.Contents) == 0 && !strings.HasSuffix(strings.ToLower(file.Path), ".map") {
				return fmt.Errorf("esbuild produced empty output for %s", file.Path)
			}
			vfsPath := filepath.Join(ctx.destDir, filepath.FromSlash(outputs[i]))

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

			if cacheRoot != nil {
				rel := filepath.FromSlash(outputs[i])
				if err := cacheRoot.MkdirAll(filepath.Dir(rel), defaultDirMode); err == nil {
					_ = cacheRoot.WriteFile(rel, contents, defaultFileMode)
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

		// Evaluate symlinks to handle symlinked theme directories correctly
		resolvedSrcDir := srcDir
		if realSrc, err := filepath.EvalSymlinks(srcDir); err == nil {
			resolvedSrcDir = realSrc
		}
		resolvedEntryPoint := entryPointAbs
		if realEntry, err := filepath.EvalSymlinks(entryPointAbs); err == nil {
			resolvedEntryPoint = realEntry
		}

		relEntryPoint, _ := SafeRel(resolvedSrcDir, NormalizePath(resolvedEntryPoint))
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

func buildEntryPoints(ctx buildAssetsContext, entryPoints []string, bundle bool, assets map[string]string, assetsMu *sync.Mutex, cachePath string, outputs *[]string) error {
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

	paths, err := esbuildOutputPaths(ctx.destDir, result.OutputFiles)
	if err != nil {
		return err
	}
	assetsMu.Lock()
	*outputs = append(*outputs, paths...)
	assetsMu.Unlock()
	return updateAssetsFromMetafile(result.Metafile, ctx.srcDir, assets, assetsMu)
}

func saveAssetCache(cachePath string, assets map[string]string, outputs []string, minify bool) {
	root := openAssetCacheRoot(cachePath)
	if root == nil {
		return
	}
	defer func() { _ = root.Close() }()
	slices.Sort(outputs)
	outputs = slices.Compact(outputs)
	mapData, _ := json.Marshal(assets)
	if err := root.WriteFile("map.json", mapData, defaultFileMode); err != nil {
		return
	}
	manifestData, _ := json.Marshal(assetCacheManifest{Version: 1, Minify: minify, Outputs: outputs})
	_ = root.WriteFile("outputs.json", manifestData, defaultFileMode)
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
	outputs := make([]string, 0)
	var assetsMu sync.Mutex

	cachePath := ""
	if ctx.cachePath != "" && !opts.Force {
		cachePath = filepath.Join(ctx.cachePath, scan.hash)
	}

	buildGroup, _ := errgroup.WithContext(context.Background())

	if scan.hasCSS() {
		buildGroup.Go(func() error {
			return buildEntryPoints(ctx, scan.points.css, true, assets, &assetsMu, cachePath, &outputs)
		})
	}
	if scan.hasJS() {
		buildGroup.Go(func() error {
			return buildEntryPoints(ctx, scan.points.js, true, assets, &assetsMu, cachePath, &outputs)
		})
	}

	if err := buildGroup.Wait(); err != nil {
		return nil, err
	}

	if cachePath != "" {
		saveAssetCache(cachePath, assets, outputs, ctx.minify)
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
