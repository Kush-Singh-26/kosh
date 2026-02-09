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

// SyncVFS synchronizes the `targetDir` directory from VFS to disk using parallel workers.
// If dirtyFiles is not nil, it only syncs files present in the map.
func SyncVFS(srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool) error {
	fmt.Println("💾 Syncing in-memory filesystem to disk...")

	targetDirClean := filepath.Clean(targetDir)

	// 1. Collect all files from VFS
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

		// Differential Sync: Only sync if dirtyFiles is nil (full sync)
		// or if the specific file is in the dirty map.
		// Always sync .nojekyll, sitemap, rss, search_index, manifest, sw.js, graph.json
		alwaysSync := map[string]bool{
			"public/.nojekyll":               true,
			"public/sitemap.xml":             true,
			"public/rss.xml":                 true,
			"public/search_index.json":       true,
			"public/search.bin":              true,
			"public/manifest.json":           true,
			"public/sw.js":                   true,
			"public/graph.json":              true,
			"public/static/search.wasm":      true,
			"public/static/wasm/search.wasm": true,
		}

		if dirtyFiles != nil {
			pathNormalized := filepath.ToSlash(path)
			// Always sync static assets and specific global files
			isStatic := strings.HasPrefix(pathNormalized, "public/static/")
			if !dirtyFiles[pathNormalized] && !alwaysSync[pathNormalized] && !isStatic {
				return nil
			}
		}

		filesToSync = append(filesToSync, path)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan VFS: %w", err)
	}

	// 2. Parallel Sync with Worker Pool
	numWorkers := runtime.NumCPU() * 2
	if numWorkers > 64 {
		numWorkers = 64
	}

	var wg sync.WaitGroup
	fileChan := make(chan string, len(filesToSync))
	errChan := make(chan error, len(filesToSync))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				if err := syncSingleFile(srcFs, path); err != nil {
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

	if len(errChan) > 0 {
		return <-errChan
	}

	return nil
}

// Global map to track created directories in this process to avoid redundant MkdirAll
var (
	createdDirs   = make(map[string]bool)
	createdDirsMu sync.RWMutex
)

func syncSingleFile(srcFs afero.Fs, path string) error {
	// Read source content ONCE
	srcContent, err := afero.ReadFile(srcFs, path)
	if err != nil {
		return err
	}

	// Check if destination exists with same content
	destContent, err := os.ReadFile(path)
	if err == nil && bytes.Equal(srcContent, destContent) {
		return nil // Identical - skip write
	}

	// Destination missing or different - write it
	// Ensure directory exists (Optimized)
	dir := filepath.Dir(path)

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

	return os.WriteFile(path, srcContent, 0644)
}
