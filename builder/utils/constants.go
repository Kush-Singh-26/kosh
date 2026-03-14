package utils

import (
	"path/filepath"
	"runtime"
)

// alwaysSyncPaths contains paths that should always be synced regardless of dirty state
var alwaysSyncPaths = map[string]bool{
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

func IsAlwaysSyncPath(relPath string) bool {
	return alwaysSyncPaths[filepath.ToSlash(relPath)]
}

// Default constants - these are used as fallbacks
// Actual values come from BuildConfig loaded from kosh.build.yaml
const (
	MaxBufferSize       = 64 * 1024 // 64KB
	InlineHTMLThreshold = 32 * 1024 // 32KB
	RawThreshold        = 512
	FastZstdMax         = 64 * 1024        // 64KB
	MaxFileSize         = 50 * 1024 * 1024 // 50MB
)

var TestingMode = false

// Legacy constant for backward compatibility
const DefaultWorkerCountMax = 32

func GetDefaultWorkerCount() int {
	workers := runtime.NumCPU()
	if workers < 2 {
		return 2
	}
	if workers > DefaultWorkerCountMax {
		return DefaultWorkerCountMax
	}
	return workers
}
