package clean

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run removes the target directory (default: "public")
func Run() {
	target := "public"
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	absTarget := filepath.Join(cwd, target)
	fmt.Printf("🧹 Cleaning '%s' directory...\n", absTarget)

	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		fmt.Printf("✅ Directory '%s' does not exist. Nothing to clean.\n", target)
		return
	}

	err = os.RemoveAll(absTarget)
	if err != nil {
		fmt.Printf("❌ Failed to remove '%s': %v\n", target, err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully cleaned '%s'.\n", target)
}
