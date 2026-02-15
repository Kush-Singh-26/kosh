package utils

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/afero"
)

var (
	createdDirs   = make(map[string]bool)
	createdDirsMu sync.RWMutex

	// File content cache to avoid redundant disk reads during sync
	fileContentCache   = make(map[string][]byte)
	fileContentCacheMu sync.RWMutex
	maxCacheEntries    = 1000
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
	"static/search.wasm":      true,
	"static/wasm/search.wasm": true,
}

func SyncVFS(srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool) error {
	fmt.Println("💾 Syncing in-memory filesystem to disk...")

	targetDirClean := filepath.Clean(targetDir)

	var filesToSync []string
	err := afero.Walk(srcFs, targetDirClean, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(path, 0755)
		}

		pathNormalized := filepath.ToSlash(path)

		if dirtyFiles != nil {
			relPath, relErr := filepath.Rel(targetDirClean, path)
			if relErr != nil {
				relPath = pathNormalized
			}
			relPath = filepath.ToSlash(relPath)

			isAlwaysSync := alwaysSyncPaths[relPath]
			isStatic := strings.HasPrefix(relPath, "static/")
			isMarkdown := strings.HasSuffix(relPath, ".md")
			isDirty := dirtyFiles[pathNormalized]

			if !isDirty && !isAlwaysSync && !isStatic && !isMarkdown {
				return nil
			}
		}

		filesToSync = append(filesToSync, path)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan VFS: %w", err)
	}

	numWorkers := runtime.NumCPU() * 2
	if numWorkers > 32 {
		numWorkers = 32
	}

	var wg sync.WaitGroup
	fileChan := make(chan string, min(len(filesToSync), 100))
	errChan := make(chan error, len(filesToSync))
	var firstErr error
	var errOnce sync.Once

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				if err := syncSingleFile(srcFs, path); err != nil {
					errOnce.Do(func() { firstErr = err })
					errChan <- err
				}
			}
		}()
	}

	for _, f := range filesToSync {
		fileChan <- f
	}
	close(fileChan)
	wg.Wait()
	close(errChan)

	if firstErr != nil {
		return firstErr
	}

	return nil
}

func syncSingleFile(srcFs afero.Fs, path string) error {
	srcContent, err := afero.ReadFile(srcFs, path)
	if err != nil {
		return err
	}

	osPath := filepath.FromSlash(path)

	// Check content cache first
	fileContentCacheMu.RLock()
	cached, inCache := fileContentCache[osPath]
	fileContentCacheMu.RUnlock()

	if inCache && bytes.Equal(srcContent, cached) {
		return nil // Skip write, content unchanged from cache
	}

	// Only stat if not in cache
	destContent, err := os.ReadFile(osPath)
	if err == nil && bytes.Equal(srcContent, destContent) {
		// Update cache with matched content
		fileContentCacheMu.Lock()
		if len(fileContentCache) >= maxCacheEntries {
			// Simple eviction: clear half
			cnt := 0
			for k := range fileContentCache {
				delete(fileContentCache, k)
				cnt++
				if cnt >= maxCacheEntries/2 {
					break
				}
			}
		}
		fileContentCache[osPath] = srcContent
		fileContentCacheMu.Unlock()
		return nil
	}

	dir := filepath.Dir(osPath)

	createdDirsMu.RLock()
	exists := createdDirs[dir]
	createdDirsMu.RUnlock()

	if !exists {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		createdDirsMu.Lock()
		createdDirs[dir] = true
		createdDirsMu.Unlock()
	}

	if err := os.WriteFile(osPath, srcContent, 0644); err != nil {
		return err
	}

	// Update cache after successful write
	fileContentCacheMu.Lock()
	fileContentCache[osPath] = srcContent
	fileContentCacheMu.Unlock()

	return nil
}
