package clean

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Run removes the target directory (default: "public")
// If cleanCache is true, also removes the .kosh-cache directory
func Run(cleanCache bool) {
	start := time.Now()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	failed := false

	// Helper to clean a directory
	cleanDir := func(name string) {
		defer wg.Done()
		dirStart := time.Now()
		absPath := filepath.Join(cwd, name)
		fmt.Printf("🧹 Cleaning '%s'...\n", absPath)

		if err := os.RemoveAll(absPath); err != nil {
			mu.Lock()
			failed = true
			// improved error message for locked files (common on Windows)
			if os.IsPermission(err) {
				fmt.Printf("   ❌ Failed to remove '%s': %v\n", name, err)
				fmt.Printf("   💡 Hint: Is the server still running? Stop 'kosh serve' and try again.\n")
			} else {
				fmt.Printf("   ❌ Failed to remove '%s': %v\n", name, err)
			}
			mu.Unlock()
			return
		}
		fmt.Printf("   ✅ Cleaned '%s' in %v.\n", name, time.Since(dirStart))
	}

	// 1. Clean public directory
	wg.Add(1)
	go cleanDir("public")

	// 2. Clean cache directory if requested
	if cleanCache {
		wg.Add(1)
		go cleanDir(".kosh-cache")
	}

	wg.Wait()

	if failed {
		os.Exit(1)
	}

	fmt.Printf("🧹 Clean completed in %v.\n", time.Since(start))
}
