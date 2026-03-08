package renderer

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	hashes      map[string]string
	templateDir string
	mu          sync.RWMutex
	lastCheckNs atomic.Int64 // UnixNano of last TTL check; atomic for lock-free reads
	checkTTL    time.Duration
}

var (
	globalCache   *templateCache
	globalCacheMu sync.Mutex
)

func getGlobalCache(templateDir string, devMode bool) *templateCache {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	ttl := 2 * time.Second
	if devMode {
		ttl = 100 * time.Millisecond
	}

	if globalCache == nil || globalCache.templateDir != templateDir {
		globalCache = &templateCache{
			templates:   make(map[string]*template.Template),
			mtimes:      make(map[string]time.Time),
			hashes:      make(map[string]string),
			templateDir: templateDir,
			checkTTL:    ttl,
		}
	} else {
		// Update TTL in case devMode changed (though unlikely)
		globalCache.mu.Lock()
		globalCache.checkTTL = ttl
		globalCache.mu.Unlock()
	}

	return globalCache
}

func (tc *templateCache) hasTemplatesChanged() bool {
	now := time.Now()
	nowNs := now.UnixNano()
	checkTTLNs := tc.checkTTL.Nanoseconds()

	// Load the last check time atomically — no lock needed for TTL comparison
	lastNs := tc.lastCheckNs.Load()
	if nowNs-lastNs < checkTTLNs {
		return false
	}
	// CAS to prevent stampede: only one goroutine proceeds past TTL at a time
	if !tc.lastCheckNs.CompareAndSwap(lastNs, nowNs) {
		return false
	}

	templateFiles := []string{"layout.html", "index.html", "graph.html", "404.html"}
	changed := false

	// Use read lock to check metadata first
	tc.mu.RLock()
	for _, fname := range templateFiles {
		path := filepath.Join(tc.templateDir, fname)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(fname, ".html")
		cachedMtime, exists := tc.mtimes[name]

		if !exists || !info.ModTime().Equal(cachedMtime) {
			changed = true
			break
		}
	}
	tc.mu.RUnlock()

	return changed
}

func (tc *templateCache) setTemplate(name string, tmpl *template.Template, mtime time.Time, content []byte) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.templates[name] = tmpl
	tc.mtimes[name] = mtime
	if len(content) > 0 {
		tc.hashes[name] = cache.HashContent(content)
	}
}
