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

	if cleanAllVersions {
		cleanDirAsync(cwd, "public")
	} else {
		cleanRootFilesOnly(cwd)
	}

	if cleanCache {
		cleanDirAsync(cwd, ".kosh-cache")
	}

	fmt.Printf("🧹 Clean initiated in %v (backgrounding deletion).\n", time.Since(start))
}

func cleanDirAsync(cwd, name string) {
	absPath := filepath.Join(cwd, name)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return
	}

	tempName := fmt.Sprintf("%s_deleting_%d", name, time.Now().UnixNano())
	tempPath := filepath.Join(cwd, tempName)

	fmt.Printf("🧹 Moving '%s' to trash...\n", name)
	if err := os.Rename(absPath, tempPath); err != nil {
		fmt.Printf("⚠️ Rename failed (%v), deleting synchronously...\n", err)
		if err := os.RemoveAll(absPath); err != nil {
			fmt.Printf("❌ Failed to remove '%s': %v\n", name, err)
		}
		return
	}

	go func() {
		_ = os.RemoveAll(tempPath)
	}()
}

func cleanRootFilesOnly(cwd string) {
	publicPath := filepath.Join(cwd, "public")
	if _, err := os.Stat(publicPath); os.IsNotExist(err) {
		return
	}

	cfg := config.Load([]string{})
	if cfg == nil {
		fmt.Println("⚠️ Failed to load config, cleaning entire public/ directory")
		cleanDirAsync(cwd, "public")
		return
	}

	preservePaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			preservePaths[v.Path] = true
		}
	}

	if len(preservePaths) == 0 {
		fmt.Println("🧹 No versions configured, cleaning entire public/ directory")
		cleanDirAsync(cwd, "public")
		return
	}

	files, err := os.ReadDir(publicPath)
	if err != nil {
		fmt.Printf("❌ Failed to read public directory: %v\n", err)
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
		itemPath := filepath.Join(publicPath, name)
		tempName := fmt.Sprintf("%s_deleting_%d", name, time.Now().UnixNano())
		tempPath := filepath.Join(publicPath, tempName)

		if err := os.Rename(itemPath, tempPath); err != nil {
			_ = os.RemoveAll(itemPath)
			continue
		}

		go func(tp string) {
			_ = os.RemoveAll(tp)
		}(tempPath)
	}
}
