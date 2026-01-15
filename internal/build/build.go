package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckWASM checks if the search engine WASM needs to be rebuilt and builds it if necessary.
func CheckWASM() {
	// Ensure bin directory exists (though not strictly needed for WASM)
	_ = os.MkdirAll("bin", 0755)

	wasmSrc := []string{
		"cmd/search",
		"builder/search",
		"builder/models",
	}
	wasmOut := "static/wasm/search.wasm"

	if needsRebuild(wasmSrc, wasmOut) {
		fmt.Println("🚀 WASM source changes detected. Building Search WASM...")
		cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", wasmOut, "./cmd/search/main.go")
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ WASM build failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ WASM build complete.")
	}
}

func needsRebuild(srcs []string, out string) bool {
	outInfo, err := os.Stat(out)
	if err != nil {
		return true // Output file doesn't exist
	}
	outTime := outInfo.ModTime()

	for _, src := range srcs {
		err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && info.ModTime().After(outTime) {
				return fmt.Errorf("rebuild needed")
			}
			return nil
		})
		if err != nil {
			return true
		}
	}
	return false
}
