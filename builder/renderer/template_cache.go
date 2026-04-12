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
	"github.com/spf13/afero"
	"golang.org/x/sync/singleflight"
)

const (
	templateCacheTTL    = 2 * time.Second
	templateCacheDevTTL = 100 * time.Millisecond
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	hashes      map[string]string
	templateDir string
	mu          sync.RWMutex // protects templates, mtimes, hashes, checkTTL
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

	ttl := templateCacheTTL
	if devMode {
		ttl = templateCacheDevTTL
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

func (tc *templateCache) hasTemplatesChanged(fs afero.Fs) bool {
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
	result, err, _ := tc.sf.Do("checkTemplates", func() (any, error) {
		// Re-check TTL after acquiring singleflight to handle race condition
		nowNs := time.Now().UnixNano()
		lastNs := tc.lastCheckNs.Load()
		if nowNs-lastNs < checkTTLNs {
			// Another goroutine already checked within TTL
			return false, nil
		}

		// Update last check time
		tc.lastCheckNs.Store(nowNs)

		changed, _ := tc.checkTemplatesOnDisk(fs)
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

func (tc *templateCache) checkTemplatesOnDisk(fs afero.Fs) (bool, error) {
	templateFiles := []string{"layout.html", "index.html", "404.html"}
	changed := false

	// Add partials to the list of files to check
	partialsDir := filepath.Join(tc.templateDir, "partials")
	if info, err := fs.Stat(partialsDir); err == nil && info.IsDir() {
		// Walk partials to detect changes in snippets
		err = afero.Walk(fs, partialsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".html") {
				return nil
			}
			rel, err := filepath.Rel(tc.templateDir, path)
			if err == nil {
				templateFiles = append(templateFiles, rel)
			}
			return nil
		})
	}

	// Use read lock to check metadata first
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	for _, fname := range templateFiles {
		path := filepath.Join(tc.templateDir, fname)
		info, err := fs.Stat(path)
		if err != nil {
			// If a previously tracked template is gone, that's a change
			name := strings.TrimSuffix(filepath.ToSlash(fname), ".html")
			if _, exists := tc.mtimes[name]; exists {
				changed = true
				break
			}
			continue
		}

		name := strings.TrimSuffix(filepath.ToSlash(fname), ".html")
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
