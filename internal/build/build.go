package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckWASM checks if the search engine WASM needs to be rebuilt based on source hash.
func CheckWASM(currentHash string) bool {
	wasmOut := "static/wasm/search.wasm"
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		fmt.Printf("⚠️ Failed to create WASM directory: %v\n", err)
	}

	_, err := os.Stat(wasmOut)
	if err == nil && currentHash != "" {
		// We have a hash and the file exists, we can trust the hash check done by the caller
		return false
	}

	fmt.Println("🚀 Building Search WASM...")
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", wasmOut, "./cmd/search/main.go")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ WASM build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ WASM build complete.")
	return true
}
