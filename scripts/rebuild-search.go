package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/andybalholm/brotli"
)

func main() {
	// 1. Setup paths
	repoRoot, err := getRepoRoot()
	if err != nil {
		fmt.Printf("Error finding repo root: %v\n", err)
		os.Exit(1)
	}

	cmdSearch := filepath.Join(repoRoot, "cmd", "search")
	wasmDest := filepath.Join(repoRoot, "search.wasm")
	brDest := filepath.Join(repoRoot, "builder", "assets", "wasm", "search.wasm.br")

	fmt.Printf("Building Search WASM...\n")

	// 2. Build WASM
	cmd := exec.Command("go", "build", "-o", wasmDest, cmdSearch)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}

	// 3. Compress with Brotli
	fmt.Printf("Compressing with Brotli (Best Compression)...\n")
	data, err := os.ReadFile(wasmDest)
	if err != nil {
		fmt.Printf("Failed to read built WASM: %v\n", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	bw := brotli.NewWriterLevel(&buf, brotli.BestCompression)
	if _, err := bw.Write(data); err != nil {
		fmt.Printf("Compression failed: %v\n", err)
		os.Exit(1)
	}
	if err := bw.Close(); err != nil {
		fmt.Printf("Failed to close Brotli writer: %v\n", err)
		os.Exit(1)
	}

	// 4. Write to builder/assets/wasm/
	if err := os.MkdirAll(filepath.Dir(brDest), 0755); err != nil {
		fmt.Printf("Failed to create destination directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(brDest, buf.Bytes(), 0644); err != nil {
		fmt.Printf("Failed to write compressed WASM: %v\n", err)
		os.Exit(1)
	}

	// 5. Cleanup
	_ = os.Remove(wasmDest)

	fmt.Printf("Successfully rebuilt and compressed search engine!\n")
	fmt.Printf("  Uncompressed: %s\n", formatSize(len(data)))
	fmt.Printf("  Compressed:   %s\n", formatSize(buf.Len()))
	fmt.Printf("  Output:       %s\n", brDest)
}

func getRepoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not get caller info")
	}
	// scripts/rebuild-search.go -> go back one level
	return filepath.Dir(filepath.Dir(filename)), nil
}

func formatSize(size int) string {
	return fmt.Sprintf("%.2f KB", float64(size)/1024)
}
