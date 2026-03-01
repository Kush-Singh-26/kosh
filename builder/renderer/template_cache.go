package renderer

import (
	"html/template"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	hashes      map[string]string
	templateDir string
	mu          sync.RWMutex
	lastCheck   time.Time
	checkTTL    time.Duration
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
			hashes:      make(map[string]string),
			templateDir: templateDir,
			checkTTL:    2 * time.Second,
		}
	})
	return globalCache
}

func (tc *templateCache) hasTemplatesChanged() bool {
	now := time.Now()

	tc.mu.RLock()
	if now.Sub(tc.lastCheck) < tc.checkTTL {
		tc.mu.RUnlock()
		return false
	}
	tc.mu.RUnlock()

	templateFiles := []string{"layout.html", "index.html", "graph.html", "404.html"}
	changed := false

	for _, fname := range templateFiles {
		path := filepath.Join(tc.templateDir, fname)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		hash := cache.HashContent(content)

		tc.mu.RLock()
		cachedHash, exists := tc.hashes[fname]
		tc.mu.RUnlock()

		if !exists || cachedHash != hash {
			changed = true
			break
		}
	}

	tc.mu.Lock()
	tc.lastCheck = now
	tc.mu.Unlock()

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
