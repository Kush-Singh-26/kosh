package build

import (
	"compress/gzip"
	"fmt"
	"io"
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

	// Compress WASM
	fmt.Println("📦 Compressing WASM...")
	if err := compressGzip(wasmOut, wasmOut+".gz"); err != nil {
		fmt.Printf("⚠️ Failed to compress WASM: %v\n", err)
	} else {
		fmt.Printf("✅ WASM compressed size: %s\n", getFileSize(wasmOut+".gz"))
	}
	return true
}

func compressGzip(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	_, err = io.Copy(gw, in)
	return err
}

func getFileSize(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%.2f KB", float64(fi.Size())/1024)
}
