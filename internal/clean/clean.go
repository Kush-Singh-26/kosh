package clean

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run removes the target directory (default: "public")
// If cleanCache is true, also removes the .kosh-cache directory
func Run(cleanCache bool) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	// Clean public directory
	target := "public"
	absTarget := filepath.Join(cwd, target)
	fmt.Printf("🧹 Cleaning '%s' directory...\n", absTarget)

	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		fmt.Printf("✅ Directory '%s' does not exist. Nothing to clean.\n", target)
	} else {
		err = os.RemoveAll(absTarget)
		if err != nil {
			fmt.Printf("❌ Failed to remove '%s': %v\n", target, err)
			os.Exit(1)
		}
		fmt.Printf("✅ Successfully cleaned '%s'.\n", target)
	}

	// Clean cache directory if requested
	if cleanCache {
		cacheDir := ".kosh-cache"
		absCache := filepath.Join(cwd, cacheDir)
		fmt.Printf("🧹 Cleaning '%s' directory...\n", absCache)

		if _, err := os.Stat(absCache); os.IsNotExist(err) {
			fmt.Printf("✅ Directory '%s' does not exist. Nothing to clean.\n", cacheDir)
		} else {
			err = os.RemoveAll(absCache)
			if err != nil {
				fmt.Printf("❌ Failed to remove '%s': %v\n", cacheDir, err)
				os.Exit(1)
			}
			fmt.Printf("✅ Successfully cleaned '%s'.\n", cacheDir)
		}
	}
}
