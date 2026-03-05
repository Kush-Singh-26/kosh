package utils

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
)

var (
	createdDirs *lru.Cache[string, bool]

	// File content cache to avoid redundant disk reads during sync
	fileContentCache *lru.Cache[string, []byte]
)

func init() {
	var err error
	createdDirs, err = lru.New[string, bool](2000)
	if err != nil {
		panic("failed to create createdDirs cache: " + err.Error())
	}

	fileContentCache, err = lru.New[string, []byte](1000)
	if err != nil {
		panic("failed to create fileContentCache: " + err.Error())
	}
}

const (
	WriteBufferSize = 64 * 1024 // 64KB buffer for writes
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
	"graph.html":              true,
	"static/search.wasm":      true,
	"static/wasm/search.wasm": true,
}

func IsAlwaysSyncPath(relPath string) bool {
	return alwaysSyncPaths[filepath.ToSlash(relPath)]
}

func ClearSyncCache() {
	fileContentCache.Purge()
	createdDirs.Purge()
}

type syncTask struct {
	srcPath  string // Path in srcFs (In-memory)
	destPath string // Path on OS disk (Physical)
}

func SyncVFS(ctx context.Context, srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool, isCleanBuild bool) error {
	slog.Info("Syncing in-memory filesystem to disk", "clean_build", isCleanBuild)

	targetDirClean := filepath.Clean(targetDir)
	tx := NewTxSync(slog.Default())
	defer func() {
		if !tx.committed {
			tx.Rollback()
		}
	}()

	// 1. Collect all files to sync and resolve absolute/relative paths
	var tasks []syncTask
	seen := make(map[string]bool)

	// Files rendered in this build
	for path := range dirtyFiles {
		srcP := path
		destP := path
		if !filepath.IsAbs(destP) {
			destP = filepath.Join(targetDirClean, filepath.FromSlash(destP))
		}

		normalizedDest := filepath.Clean(destP)
		if !seen[normalizedDest] {
			tasks = append(tasks, syncTask{srcPath: srcP, destPath: normalizedDest})
			seen[normalizedDest] = true
		}
	}

	// Always sync paths (configs, bin, etc)
	for path := range alwaysSyncPaths {
		// Check both root-relative and output-relative paths in VFS
		srcP := path
		destP := filepath.Join(targetDirClean, filepath.FromSlash(path))
		normalizedDest := filepath.Clean(destP)

		if !seen[normalizedDest] {
			// Check if it exists in VFS at root or with output prefix
			exists := false
			if ex, _ := afero.Exists(srcFs, srcP); ex {
				exists = true
			} else if ex, _ := afero.Exists(srcFs, filepath.Join(targetDirClean, srcP)); ex {
				exists = true
				srcP = filepath.Join(targetDirClean, srcP)
			}

			if exists {
				tasks = append(tasks, syncTask{srcPath: srcP, destPath: normalizedDest})
				seen[normalizedDest] = true
			}
		}
	}

	if len(tasks) == 0 {
		return nil
	}

	// 2. Pre-create directories in parallel
	dirs := make(map[string]bool)
	for _, task := range tasks {
		dirs[filepath.Dir(task.destPath)] = true
	}

	dirPool := NewWorkerPool(ctx, runtime.NumCPU(), func(dir string) {
		_ = os.MkdirAll(dir, 0755)
	})
	dirPool.Start()
	for dir := range dirs {
		dirPool.Submit(dir)
	}
	dirPool.Stop()

	// 3. Sync files with high concurrency
	numWorkers := runtime.NumCPU() * 2
	if numWorkers > 32 {
		numWorkers = 32
	}

	pool := NewWorkerPool(ctx, numWorkers, func(task syncTask) {
		if err := syncSingleFileTask(srcFs, task, isCleanBuild, tx); err != nil {
			slog.Error("Error syncing file", "path", task.destPath, "error", err)
		}
	})
	pool.Start()

	for _, task := range tasks {
		pool.Submit(task)
	}
	pool.Stop()

	tx.Commit()
	return nil
}

func syncSingleFileTask(srcFs afero.Fs, task syncTask, isCleanBuild bool, tx *TxSync) error {
	// Use ToSlash for VFS lookups to ensure consistency on Windows
	srcContent, err := afero.ReadFile(srcFs, filepath.ToSlash(task.srcPath))
	if err != nil {
		// Try fallback if path was already OS-specific
		srcContent, err = afero.ReadFile(srcFs, task.srcPath)
		if err != nil {
			// If file is missing from VFS, it's not necessarily an error.
			// It might be a cached file that we just want to protect from the orphan cleaner.
			// We only error if we are sure it SHOULD be in the VFS.
			return nil
		}
	}

	osPath := task.destPath

	if !isCleanBuild {
		// Check content cache first (LRU is thread-safe)
		cached, inCache := fileContentCache.Get(osPath)

		if inCache && bytes.Equal(srcContent, cached) {
			return nil // Skip write, content unchanged from cache
		}

		// Fast Metadata Check: if file exists and size differs, it's dirty
		info, err := os.Stat(osPath)
		if err == nil {
			if info.Size() != int64(len(srcContent)) {
				// Size differs, definitely dirty
			} else {
				// Size matches, do a content check
				destContent, err := os.ReadFile(osPath)
				if err == nil && bytes.Equal(srcContent, destContent) {
					// Update cache with matched content
					fileContentCache.Add(osPath, srcContent)
					return nil
				}
			}
		}
	}

	// LAZY BACKUP: only track write (which creates a backup) if we are actually writing
	if tx != nil {
		if err := tx.TrackWrite(osPath); err != nil {
			return fmt.Errorf("failed to track write for rollback: %w", err)
		}
	}

	if err := atomicWrite(osPath, srcContent); err != nil {
		return err
	}

	// Update cache after successful write
	fileContentCache.Add(osPath, srcContent)

	return nil
}

// atomicWrite writes data to a temporary file and then renames it to the target path.
// This ensures that the target file is either fully written or not written at all.
func atomicWrite(path string, data []byte) error {
	// Ensure parent directory exists (handles race conditions with background clean)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	writer := SharedBufioWriterPool.Get(f)
	_, err = writer.Write(data)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		SharedBufioWriterPool.Put(writer)
		return err
	}

	err = writer.Flush()
	SharedBufioWriterPool.Put(writer)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	err = f.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Atomic rename
	err = os.Rename(tmpPath, path)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
