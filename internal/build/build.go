package build

import (
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed wasm/search.wasm
var searchWasm []byte

// CheckWASM ensures the search engine WASM is present in the output directory.
func CheckWASM(_ string) bool {
	wasmOut := "static/wasm/search.wasm"
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		fmt.Printf("⚠️ Failed to create WASM directory: %v\n", err)
	}

	// Check if file exists
	if _, err := os.Stat(wasmOut); err == nil {
		// Optimization: Assume embedded WASM doesn't change frequently during dev
		// We could check hash here but for now existence is enough to avoid re-write
		return false
	}

	fmt.Println("🚀 Writing embedded Search WASM...")
	if err := os.WriteFile(wasmOut, searchWasm, 0644); err != nil {
		fmt.Printf("❌ Failed to write WASM: %v\n", err)
		os.Exit(1)
	}

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
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	gw := gzip.NewWriter(out)
	defer func() { _ = gw.Close() }()

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
