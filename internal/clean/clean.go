package clean

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

// cleanupWg tracks background deletion goroutines for proper shutdown
var cleanupWg sync.WaitGroup

func Run(cleanCache, cleanAllVersions bool) {
	RunFs(afero.NewOsFs(), cleanCache, cleanAllVersions)
}

func RunFs(fs afero.Fs, cleanCache, cleanAllVersions bool) {
	start := time.Now()

	// Get outputDir from config (fallback to "public")
	outputDir := "public"
	cfg := config.Load([]string{})
	if cfg != nil && cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	if cleanAllVersions {
		cleanDirAsync(fs, outputDir)
	} else {
		cleanRootFilesOnly(fs, outputDir, cfg)
	}

	if cleanCache {
		cacheDir := ".kosh-cache"
		if cfg != nil && cfg.CacheDir != "" {
			cacheDir = cfg.CacheDir
		}
		cleanDirAsync(fs, cacheDir)
	}

	run.DevLogInfo(fmt.Sprintf("Clean initiated in %v.", time.Since(start)))
}

// WaitForCleanup blocks until all background cleanup goroutines complete.
// Call this before program exit if you need to ensure cleanup completes.
func WaitForCleanup() {
	cleanupWg.Wait()
}

func cleanDirAsync(fs afero.Fs, path string) {
	exists, _ := afero.Exists(fs, path)
	if !exists {
		return
	}

	if utils.TestingMode {
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

	fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[96mℹ\033[0m  Moving '%s' to trash...\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), path)
	if err := fs.Rename(path, tempPath); err != nil {
		fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[91m✗\033[0m  Rename failed (%v), deleting synchronously...\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), err)
		if err := fs.RemoveAll(path); err != nil {
			fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[91m✗\033[0m  Failed to remove '%s': %v\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), path, err)
		}
		return
	}

	cleanupWg.Add(1)
	go func() {
		defer cleanupWg.Done()
		_ = fs.RemoveAll(tempPath)
	}()
}

func cleanRootFilesOnly(fs afero.Fs, outputDir string, cfg *config.Config) {
	exists, _ := afero.Exists(fs, outputDir)
	if !exists {
		return
	}

	if cfg == nil {
		fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[96mℹ\033[0m  Failed to load config, cleaning entire %s directory\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), outputDir)
		cleanDirAsync(fs, outputDir)
		return
	}

	preservePaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			preservePaths[v.Path] = true
		}
	}

	if len(preservePaths) == 0 {
		fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[96mℹ\033[0m  No versions configured, cleaning entire %s directory\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), outputDir)
		cleanDirAsync(fs, outputDir)
		return
	}

	files, err := afero.ReadDir(fs, outputDir)
	if err != nil {
		fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[91m✗\033[0m  Failed to read output directory: %v\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), err)
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
		fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[96mℹ\033[0m  No files to clean (only version folders present)\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second())
		return
	}

	fmt.Printf("\033[90m%02d:%02d:%02d\033[0m \033[96mℹ\033[0m  Cleaning root files (%d items), preserving %d version folders...\n", time.Now().Hour(), time.Now().Minute(), time.Now().Second(), len(toDelete), len(preservePaths))

	for _, name := range toDelete {
		itemPath := filepath.Join(outputDir, name)
		removePathAsync(fs, itemPath)
	}
}
