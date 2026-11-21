package renderer

import (
	"errors"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sync/singleflight"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
)

const (
	templateCacheTTL    = 2 * time.Second
	templateCacheDevTTL = 100 * time.Millisecond
)

type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time // filesystem dependency paths, not template slot names
	hashes      map[string]string
	templateDir string
	layoutsDir  string
	mu          sync.RWMutex // protects templates, mtimes, hashes, checkTTL
	lastCheckNs atomic.Int64 // UnixNano of last TTL check; atomic for lock-free reads
	checkTTL    time.Duration
	sf          singleflight.Group
}

var (
	globalCache   *templateCache
	globalCacheMu sync.Mutex
)

func getGlobalCache(templateDir, layoutsDir string, devMode bool) *templateCache {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	ttl := templateCacheTTL
	if devMode {
		ttl = templateCacheDevTTL
	}

	if globalCache == nil || globalCache.templateDir != templateDir || globalCache.layoutsDir != layoutsDir {
		globalCache = &templateCache{
			templates:   make(map[string]*template.Template),
			mtimes:      make(map[string]time.Time),
			hashes:      make(map[string]string),
			templateDir: templateDir,
			layoutsDir:  layoutsDir,
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
	dependencies, err := tc.templateDependencies(fs)
	if err != nil {
		return false, err
	}

	tc.mu.RLock()
	defer tc.mu.RUnlock()
	if len(dependencies) != len(tc.mtimes) {
		return true, nil
	}
	for path, mtime := range dependencies {
		cachedMtime, exists := tc.mtimes[path]
		if !exists || !mtime.Equal(cachedMtime) {
			return true, nil
		}
	}
	return false, nil
}

// templateDependencies mirrors slot precedence and both partial loading passes.
// The graph and fallback 404 templates are embedded and have no disk dependency.
func (tc *templateCache) templateDependencies(fs afero.Fs) (map[string]time.Time, error) {
	dependencies := make(map[string]time.Time)
	for _, file := range []string{"layout.html", "index.html", "404.html", "home.html"} {
		for _, dir := range []string{tc.layoutsDir, tc.templateDir} {
			path := filepath.Join(dir, file)
			info, err := fs.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				continue
			}
			dependencies[path] = info.ModTime()
			break
		}
	}
	for _, dir := range []string{tc.templateDir, tc.layoutsDir} {
		if err := collectPartialDependencies(fs, filepath.Join(dir, "partials"), dependencies); err != nil {
			return nil, err
		}
	}
	return dependencies, nil
}

func collectPartialDependencies(fs afero.Fs, dir string, dependencies map[string]time.Time) error {
	if _, err := fs.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return afero.Walk(fs, dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".html") {
			dependencies[path] = info.ModTime()
		}
		return nil
	})
}

func (tc *templateCache) setTemplate(name string, tmpl *template.Template, content []byte) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.templates[name] = tmpl

	if len(content) > 0 {
		tc.hashes[name] = core.HashContent(content)
	}
}
