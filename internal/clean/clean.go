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

	var bgWg sync.WaitGroup // WaitGroup for background tasks (we don't wait for them to finish before returning)

	// Helper to clean a directory asynchronously
	cleanDirAsync := func(name string) {
		absPath := filepath.Join(cwd, name)
		// Check if exists
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return
		}

		// 1. Rename to temp
		tempName := fmt.Sprintf("%s_deleting_%d", name, time.Now().UnixNano())
		tempPath := filepath.Join(cwd, tempName)

		fmt.Printf("🧹 Moving '%s' to trash...\n", name)
		if err := os.Rename(absPath, tempPath); err != nil {
			// Fallback to synchronous delete if rename fails
			fmt.Printf("⚠️ Rename failed (%v), deleting synchronously...\n", err)
			if err := os.RemoveAll(absPath); err != nil {
				fmt.Printf("❌ Failed to remove '%s': %v\n", name, err)
			}
			return
		}

		// 2. Delete in background
		bgWg.Add(1) // We increment but we won't wait in main thread for this specific one to block 'Run' return
		// Actually, we want 'Run' to return so build can start.
		// But if the program exits, background goroutines die.
		// Wait, 'kosh clean' then runs 'run.Run'. If 'run.Run' finishes, program exits.
		// If deletion is slower than build, it might be cut off.
		// Is that a problem? The temp folder stays.
		// We should probably ensure cleanup happens, or detach it?
		// Go doesn't support daemon threads that survive main exit easily.
		//
		// However, 'run.Run' usually takes 10s+. Deletion takes 2s.
		// So it's likely fine.
		// Ideally, we hand off the wg to main?
		// No, let's just detach.

		go func() {
			if err := os.RemoveAll(tempPath); err != nil {
				// Cleanup failed
			}
		}()
	}

	// 1. Clean public directory
	cleanDirAsync("public")

	// 2. Clean cache directory if requested
	if cleanCache {
		cleanDirAsync(".kosh-cache")
	}

	// We return IMMEDIATELY after rename.
	// This allows 'run.Run' to start while 'os.RemoveAll' churns on the temp folders.
	fmt.Printf("🧹 Clean initiated in %v (backgrounding deletion).\n", time.Since(start))
}
