package clean

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
)

func Run(cleanCache, cleanAllVersions bool) {
	start := time.Now()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	// Get outputDir from config (fallback to "public")
	outputDir := "public"
	cfg := config.Load([]string{})
	if cfg != nil && cfg.OutputDir != "" {
		outputDir = cfg.OutputDir
	}

	// Resolve to absolute path
	var absOutputPath string
	if filepath.IsAbs(outputDir) {
		absOutputPath = outputDir
	} else {
		absOutputPath = filepath.Join(cwd, outputDir)
	}

	if cleanAllVersions {
		cleanDirAsync(absOutputPath)
	} else {
		cleanRootFilesOnly(absOutputPath, cfg)
	}

	if cleanCache {
		cachePath := filepath.Join(cwd, ".kosh-cache")
		cleanDirAsync(cachePath)
	}

	fmt.Printf("🧹 Clean initiated in %v (backgrounding deletion).\n", time.Since(start))
}

func cleanDirAsync(absPath string) {
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return
	}

	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)
	tempName := fmt.Sprintf("%s_deleting_%d", base, time.Now().UnixNano())
	tempPath := filepath.Join(dir, tempName)

	fmt.Printf("🧹 Moving '%s' to trash...\n", absPath)
	if err := os.Rename(absPath, tempPath); err != nil {
		fmt.Printf("⚠️ Rename failed (%v), deleting synchronously...\n", err)
		if err := os.RemoveAll(absPath); err != nil {
			fmt.Printf("❌ Failed to remove '%s': %v\n", absPath, err)
		}
		return
	}

	go func() {
		_ = os.RemoveAll(tempPath)
	}()
}

func cleanRootFilesOnly(absOutputPath string, cfg *config.Config) {
	if _, err := os.Stat(absOutputPath); os.IsNotExist(err) {
		return
	}

	if cfg == nil {
		fmt.Printf("⚠️ Failed to load config, cleaning entire %s/ directory\n", absOutputPath)
		cleanDirAsync(absOutputPath)
		return
	}

	preservePaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			preservePaths[v.Path] = true
		}
	}

	if len(preservePaths) == 0 {
		fmt.Printf("🧹 No versions configured, cleaning entire %s/ directory\n", absOutputPath)
		cleanDirAsync(absOutputPath)
		return
	}

	files, err := os.ReadDir(absOutputPath)
	if err != nil {
		fmt.Printf("❌ Failed to read output directory: %v\n", err)
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
		fmt.Println("🧹 No files to clean (only version folders present)")
		return
	}

	fmt.Printf("🧹 Cleaning root files (%d items), preserving %d version folders...\n", len(toDelete), len(preservePaths))

	for _, name := range toDelete {
		itemPath := filepath.Join(absOutputPath, name)
		tempName := fmt.Sprintf("%s_deleting_%d", name, time.Now().UnixNano())
		tempPath := filepath.Join(absOutputPath, tempName)

		if err := os.Rename(itemPath, tempPath); err != nil {
			_ = os.RemoveAll(itemPath)
			continue
		}

		go func(tp string) {
			_ = os.RemoveAll(tp)
		}(tempPath)
	}
}
