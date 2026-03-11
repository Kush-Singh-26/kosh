package clean

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
)

var testingMode = false

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

	fmt.Printf("Clean initiated in %v.\n", time.Since(start))
}

func cleanDirAsync(fs afero.Fs, path string) {
	exists, _ := afero.Exists(fs, path)
	if !exists {
		return
	}

	if testingMode {
		_ = fs.RemoveAll(path)
		return
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tempName := fmt.Sprintf("%s_deleting_%d", base, time.Now().UnixNano())
	tempPath := filepath.Join(dir, tempName)

	fmt.Printf("Moving '%s' to trash...\n", path)
	if err := fs.Rename(path, tempPath); err != nil {
		fmt.Printf("Rename failed (%v), deleting synchronously...\n", err)
		if err := fs.RemoveAll(path); err != nil {
			fmt.Printf("Failed to remove '%s': %v\n", path, err)
		}
		return
	}

	go func() {
		_ = fs.RemoveAll(tempPath)
	}()
}

func cleanRootFilesOnly(fs afero.Fs, outputDir string, cfg *config.Config) {
	exists, _ := afero.Exists(fs, outputDir)
	if !exists {
		return
	}

	if cfg == nil {
		fmt.Printf("Failed to load config, cleaning entire %s directory\n", outputDir)
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
		fmt.Printf("No versions configured, cleaning entire %s directory\n", outputDir)
		cleanDirAsync(fs, outputDir)
		return
	}

	files, err := afero.ReadDir(fs, outputDir)
	if err != nil {
		fmt.Printf("Failed to read output directory: %v\n", err)
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
		fmt.Println("No files to clean (only version folders present)")
		return
	}

	fmt.Printf("Cleaning root files (%d items), preserving %d version folders...\n", len(toDelete), len(preservePaths))

	for _, name := range toDelete {
		itemPath := filepath.Join(outputDir, name)

		if testingMode {
			_ = fs.RemoveAll(itemPath)
			continue
		}

		tempName := fmt.Sprintf("%s_deleting_%d", name, time.Now().UnixNano())
		tempPath := filepath.Join(outputDir, tempName)

		if err := fs.Rename(itemPath, tempPath); err != nil {
			_ = fs.RemoveAll(itemPath)
			continue
		}

		go func(tp string) {
			_ = fs.RemoveAll(tp)
		}(tempPath)
	}
}
