//go:build !wasm

package fs

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/retry"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
)

var (
	createdDirs     *lru.Cache[string, bool]
	createdDirsInit sync.Once
	createdDirsErr  error

	// File content cache to avoid redundant disk reads during sync
	fileContentCache     *lru.Cache[string, []byte]
	fileContentCacheInit sync.Once
	fileContentCacheErr  error
)

func getCreatedDirsCache() (*lru.Cache[string, bool], error) {
	createdDirsInit.Do(func() {
		var err error
		createdDirs, err = lru.New[string, bool](2000)
		if err != nil {
			createdDirsErr = err
		}
	})
	return createdDirs, createdDirsErr
}

func getFileContentCache() (*lru.Cache[string, []byte], error) {
	fileContentCacheInit.Do(func() {
		var err error
		fileContentCache, err = lru.New[string, []byte](1000)
		if err != nil {
			fileContentCacheErr = err
		}
	})
	return fileContentCache, fileContentCacheErr
}

const (
	// WriteBufferSize is the default buffer size used for file writes.
	WriteBufferSize = 64 * 1024 // 64KB buffer for writes
)

// ClearSyncCache clears the VFS sync caches.
func ClearSyncCache() {
	cache, err := getFileContentCache()
	if err == nil && cache != nil {
		cache.Purge()
	}
	dirs, err := getCreatedDirsCache()
	if err == nil && dirs != nil {
		dirs.Purge()
	}
}

type syncTask struct {
	srcPath  string // Path in srcFs (In-memory)
	destPath string // Path on OS disk (Physical)
}

// SyncOptions configures SyncVFS.
type SyncOptions struct {
	Ctx          context.Context
	SrcFs        afero.Fs
	TargetDir    string
	DirtyFiles   map[string]bool
	IsCleanBuild bool
}

// SyncVFS syncs a VFS to disk with transactional safety.
func SyncVFS(opts SyncOptions) error {
	ctx := opts.Ctx
	srcFs := opts.SrcFs
	targetDir := opts.TargetDir
	dirtyFiles := opts.DirtyFiles
	isCleanBuild := opts.IsCleanBuild

	slog.Info("Syncing in-memory filesystem to disk", "clean_build", isCleanBuild)

	targetDirClean := filepath.Clean(targetDir)
	tx := NewTxSync(slog.Default())
	defer func() {
		if !tx.IsCommitted() {
			tx.Rollback(ctx)
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
	for path := range models.AlwaysSyncPaths {
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

	dirPool := async.NewWorkerPool(ctx, runtime.NumCPU(), func(dir string) error {
		return os.MkdirAll(dir, 0755)
	})
	dirPool.Start()
	for dir := range dirs {
		dirPool.Submit(dir)
	}
	_ = dirPool.Stop()

	// 3. Sync files with high concurrency
	numWorkers := min(runtime.NumCPU()*2, 32)

	pool := async.NewWorkerPool(ctx, numWorkers, func(task syncTask) error {
		if err := syncSingleFileTask(SyncFileOptions{
			Ctx:          ctx,
			SrcFs:        srcFs,
			Task:         task,
			IsCleanBuild: isCleanBuild,
			Tx:           tx,
		}); err != nil {
			slog.Error("Error syncing file", "path", task.destPath, "error", err)
			return err
		}
		return nil
	})
	pool.Start()

	for _, task := range tasks {
		pool.Submit(task)
	}
	if err := pool.Stop(); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	tx.Commit()
	return nil
}

// SyncFileOptions configures a single-file sync operation.
type SyncFileOptions struct {
	Ctx          context.Context
	SrcFs        afero.Fs
	Task         syncTask
	IsCleanBuild bool
	Tx           *TxSync
}

func syncSingleFileTask(opts SyncFileOptions) error {
	ctx := opts.Ctx
	srcFs := opts.SrcFs
	task := opts.Task
	isCleanBuild := opts.IsCleanBuild
	tx := opts.Tx

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
		cache, err := getFileContentCache()
		if err == nil && cache != nil {
			cached, inCache := cache.Get(osPath)

			if inCache && bytes.Equal(srcContent, cached) {
				return nil // Skip write, content unchanged from cache
			}
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
					if cache != nil {
						cache.Add(osPath, srcContent)
					}
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

	if err := atomicWrite(ctx, osPath, srcContent); err != nil {
		return err
	}

	// Update cache after successful write
	cache, err := getFileContentCache()
	if err == nil && cache != nil {
		cache.Add(osPath, srcContent)
	}

	return nil
}

// atomicWrite writes data to a temporary file and then renames it to the target path.
// This ensures that the target file is either fully written or not written at all.
func atomicWrite(ctx context.Context, path string, data []byte) error {
	// Ensure parent directory exists (handles race conditions with background clean)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Include PID to prevent collisions from concurrent processes
	tmpPath := path + ".tmp-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	writer := pools.SharedBufioWriterPool.Get(f)
	_, err = writer.Write(data)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		pools.SharedBufioWriterPool.Put(writer)
		return err
	}

	err = writer.Flush()
	pools.SharedBufioWriterPool.Put(writer)
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
	err = retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        ctx,
		OldPath:    tmpPath,
		NewPath:    path,
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
	})
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
