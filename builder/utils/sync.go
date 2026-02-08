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
	if numWorkers > 32 {
		numWorkers = 32
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

func syncSingleFile(srcFs afero.Fs, path string) error {
	// Get source file info first
	srcInfo, err := srcFs.Stat(path)
	if err != nil {
		return err
	}

	srcSize := srcInfo.Size()

	// Check if destination exists and matches
	if destInfo, err := os.Stat(path); err == nil {
		// Quick check: different sizes mean different content
		if destInfo.Size() != srcSize {
			goto writeFile
		}

		// For small files (< 64KB), compare content directly
		const smallFileThreshold = 64 * 1024
		if srcSize < smallFileThreshold {
			srcContent, err := afero.ReadFile(srcFs, path)
			if err != nil {
				return err
			}
			destContent, err := os.ReadFile(path)
			if err == nil && bytes.Equal(destContent, srcContent) {
				return nil // Identical
			}
		} else {
			// For large files, compare modification times first
			// This avoids reading large files into memory
			if destInfo.ModTime().Equal(srcInfo.ModTime()) || destInfo.ModTime().After(srcInfo.ModTime()) {
				return nil // Destination is same age or newer
			}
		}
	}

writeFile:
	// Read and write the file
	srcContent, err := afero.ReadFile(srcFs, path)
	if err != nil {
		return err
	}

	return os.WriteFile(path, srcContent, 0644)
}
