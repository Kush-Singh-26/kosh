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
	"golang.org/x/sync/singleflight"
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	hashes      map[string]string
	templateDir string
	mu          sync.RWMutex
	lastCheckNs atomic.Int64 // UnixNano of last TTL check; atomic for lock-free reads
	checkTTL    time.Duration
	sf          singleflight.Group
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

	// Use singleflight to ensure only one goroutine checks templates
	// All concurrent callers will share the same result
	result, err, _ := tc.sf.Do("checkTemplates", func() (interface{}, error) {
		// Re-check TTL after acquiring singleflight to handle race condition
		nowNs := time.Now().UnixNano()
		lastNs := tc.lastCheckNs.Load()
		if nowNs-lastNs < checkTTLNs {
			// Another goroutine already checked within TTL
			return false, nil
		}

		// Update last check time
		tc.lastCheckNs.Store(nowNs)

		changed, _ := tc.checkTemplatesOnDisk()
		return changed, nil
	})

	if err != nil {
		return false
	}

	changed, ok := result.(bool)
	if !ok {
		return false
	}
	return changed
}

func (tc *templateCache) checkTemplatesOnDisk() (bool, error) {
	templateFiles := []string{"layout.html", "index.html", "graph.html", "404.html"}
	changed := false

	// Use read lock to check metadata first
	tc.mu.RLock()
	defer tc.mu.RUnlock()

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

	return changed, nil
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
