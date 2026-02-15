package renderer

import (
	"html/template"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	templateDir string
	mu          sync.RWMutex
	lastCheck   time.Time
	checkTTL    time.Duration // How often to re-check mtimes
}

var (
	globalCache     *templateCache
	globalCacheOnce sync.Once
)

func getGlobalCache(templateDir string) *templateCache {
	globalCacheOnce.Do(func() {
		globalCache = &templateCache{
			templates:   make(map[string]*template.Template),
			mtimes:      make(map[string]time.Time),
			templateDir: templateDir,
			checkTTL:    2 * time.Second, // Only check mtimes every 2s
		}
	})
	return globalCache
}

func (tc *templateCache) hasTemplatesChanged() bool {
	now := time.Now()

	tc.mu.RLock()
	if now.Sub(tc.lastCheck) < tc.checkTTL {
		tc.mu.RUnlock()
		return false // Skip check, assume unchanged within TTL
	}
	tc.mu.RUnlock()

	templateFiles := []string{"layout.html", "index.html", "graph.html", "404.html"}
	changed := false

	for _, fname := range templateFiles {
		path := filepath.Join(tc.templateDir, fname)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		cachedMtime, exists := tc.mtimes[fname]
		if !exists || info.ModTime().After(cachedMtime) {
			changed = true
			break
		}
	}

	tc.mu.Lock()
	tc.lastCheck = now
	tc.mu.Unlock()

	return changed
}

func (tc *templateCache) setTemplate(name string, tmpl *template.Template, mtime time.Time) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.templates[name] = tmpl
	tc.mtimes[name] = mtime
}
