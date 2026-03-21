package clean

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/spf13/afero"
)

// cleanupWg tracks background deletion goroutines for proper shutdown
var cleanupWg sync.WaitGroup

func Run(cleanCache, cleanAllVersions bool) {
	RunFs(afero.NewOsFs(), cleanCache, cleanAllVersions, buildCtx.DetectTestingMode())
}

func RunFs(fs afero.Fs, cleanCache, cleanAllVersions bool, isTesting bool) {
	start := time.Now()

	// Get outputDir from config (fallback to "public")
	outputDir := "public"
	cfg := config.Load([]string{})
	if cfg != nil && cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	if cleanAllVersions {
		cleanDirAsync(fs, outputDir, isTesting)
	} else {
		cleanRootFilesOnly(fs, outputDir, cfg, isTesting)
	}

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

func cleanRootFilesOnly(fs afero.Fs, outputDir string, cfg *config.Config, isTesting bool) {
	exists, _ := afero.Exists(fs, outputDir)
	if !exists {
		return
	}

	if cfg == nil {
		slog.Info("Failed to load config, cleaning entire directory", "dir", outputDir)
		cleanDirAsync(fs, outputDir, isTesting)
		return
	}

	preservePaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			preservePaths[v.Path] = true
		}
	}

	if len(preservePaths) == 0 {
		slog.Info("No versions configured, cleaning entire directory", "dir", outputDir)
		cleanDirAsync(fs, outputDir, isTesting)
		return
	}

	files, err := afero.ReadDir(fs, outputDir)
	if err != nil {
		slog.Error("Failed to read output directory", "error", err)
		return
	}

	var toDelete []string
	for _, f := range files {
		name := f.Name()
		if !preservePaths[name] {
			toDelete = append(toDelete, name)
		}
	}

	if len(toDelete) == 0 {
		slog.Info("No files to clean (only version folders present)")
		return
	}

	slog.Info("Cleaning root files, preserving version folders",
		"items", len(toDelete), "versions", len(preservePaths))

	for _, name := range toDelete {
		itemPath := filepath.Join(outputDir, name)
		removePathAsync(fs, itemPath)
	}
}
