package clean

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildFs "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/spf13/afero"
)

// cleanupWg tracks background deletion goroutines for proper shutdown
var cleanupWg sync.WaitGroup

func Run(cleanCache bool) {
	RunFs(afero.NewOsFs(), cleanCache, buildFs.DetectTestingMode())
}

func RunFs(fs afero.Fs, cleanCache bool, isTesting bool) {
	start := time.Now()

	// Get outputDir from config (fallback to "public")
	outputDir := "public"
	cfg := config.Load([]string{})
	if cfg != nil && cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	cleanDirAsync(fs, outputDir, isTesting)

	if cleanCache {
		cacheDir := ".kosh-cache"
		if cfg != nil && cfg.CacheDir != "" {
			cacheDir = cfg.CacheDir
		}
		cleanDirAsync(fs, cacheDir, isTesting)
	}

	orchestration.DevLogInfo(fmt.Sprintf("Clean initiated in %v.", time.Since(start)))
}

// WaitForCleanup blocks until all background cleanup goroutines complete.
// Call this before program exit if you need to ensure cleanup completes.
func WaitForCleanup() {
	cleanupWg.Wait()
}

func cleanDirAsync(fs afero.Fs, path string, isTesting bool) {
	exists, _ := afero.Exists(fs, path)
	if !exists {
		return
	}

	if isTesting {
		_ = fs.RemoveAll(path)
		return
	}

	removePathAsync(fs, path)
}

func removePathAsync(fs afero.Fs, path string) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tempName := fmt.Sprintf("%s_deleting_%d", base, time.Now().UnixNano())
	tempPath := filepath.Join(dir, tempName)

	slog.Info("Moving to trash", "path", path)
	if err := fs.Rename(path, tempPath); err != nil {
		slog.Warn("Rename failed, deleting synchronously", "error", err)
		if err := fs.RemoveAll(path); err != nil {
			slog.Error("Failed to remove path", "path", path, "error", err)
		}
		return
	}

	cleanupWg.Add(1)
	go func() {
		defer cleanupWg.Done()
		_ = fs.RemoveAll(tempPath)
	}()
}
