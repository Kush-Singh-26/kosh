package models

import (
	"path/filepath"
	"runtime"

	"github.com/Kush-Singh-26/kosh/builder/limits"
)

// AlwaysSyncPaths contains paths that should always be synced regardless of dirty state.
var AlwaysSyncPaths = map[string]bool{
	".nojekyll":               true,
	"sitemap.xml":             true,
	"sitemap/sitemap.xml":     true,
	"rss.xml":                 true,
	"search_index.json":       true,
	"search.bin":              true,
	"manifest.json":           true,
	"sw.js":                   true,
	"graph.json":              true,
	"graph.html":              true,
	"static/search.wasm":      true,
	"static/wasm/search.wasm": true,
}

// IsAlwaysSyncPath reports whether a relative path should always be synced.
func IsAlwaysSyncPath(relPath string) bool {
	return AlwaysSyncPaths[filepath.ToSlash(relPath)]
}

// Default constants
const (
	// InlineHTMLThreshold is the max size of HTML stored inline in cache.
	InlineHTMLThreshold = 32 * 1024 // 32KB
	// RawThreshold controls when raw content is stored without compression.
	RawThreshold = 512
	// FastZstdMax is the max size for fast zstd compression.
	FastZstdMax = 64 * 1024 // 64KB
	// MaxFileSize is the maximum allowed file size for processing.
	MaxFileSize = 50 * 1024 * 1024 // 50MB
)

// GetDefaultWorkerCount returns a CPU-based default worker count.
func GetDefaultWorkerCount() int {
	workers := runtime.NumCPU()
	if workers < 2 {
		return 2
	}
	if workers > limits.MaxWorkers {
		return limits.MaxWorkers
	}
	return workers
}
