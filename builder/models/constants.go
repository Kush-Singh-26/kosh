package models

import (
	"path/filepath"
	"runtime"
)

// AlwaysSyncPaths contains paths that should always be synced regardless of dirty state
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

func IsAlwaysSyncPath(relPath string) bool {
	return AlwaysSyncPaths[filepath.ToSlash(relPath)]
}

// Default constants
const (
	MaxBufferSize       = 64 * 1024 // 64KB
	InlineHTMLThreshold = 32 * 1024 // 32KB
	RawThreshold        = 512
	FastZstdMax         = 64 * 1024        // 64KB
	MaxFileSize         = 50 * 1024 * 1024 // 50MB
	MaxWorkers          = 32
	WorkerBufferSize    = 4
)

func GetDefaultWorkerCount() int {
	workers := runtime.NumCPU()
	if workers < 2 {
		return 2
	}
	if workers > MaxWorkers {
		return MaxWorkers
	}
	return workers
}
