package build

import (
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zeebo/blake3"
)

//go:embed wasm/search.wasm
var searchWasm []byte

// embeddedWasmHash caches the hash of embedded WASM (computed once at init)
var embeddedWasmHash string

func init() {
	embeddedWasmHash = hashBytes(searchWasm)
}

// CheckWASM ensures the search engine WASM is present and up-to-date.
// Uses hash comparison to avoid unnecessary writes when WASM hasn't changed.
func CheckWASM(_ string) bool {
	wasmOut := "static/wasm/search.wasm"

	if err := os.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		fmt.Printf("⚠️ Failed to create WASM directory: %v\n", err)
	}

	// Check if deployed WASM matches embedded version
	if deployedHash, err := hashFile(wasmOut); err == nil {
		if deployedHash == embeddedWasmHash {
			// Already up-to-date, skip write
			return false
		}
		fmt.Println("🔄 WASM updated, deploying new version...")
	} else {
		fmt.Println("🚀 Writing embedded Search WASM...")
	}

	// Write new WASM
	if err := os.WriteFile(wasmOut, searchWasm, 0644); err != nil {
		fmt.Printf("❌ Failed to write WASM: %v\n", err)
		os.Exit(1)
	}

	// Compress WASM
	fmt.Println("📦 Compressing WASM...")
	if err := CompressGzip(wasmOut, wasmOut+".gz"); err != nil {
		fmt.Printf("⚠️ Failed to compress WASM: %v\n", err)
	} else {
		fmt.Printf("✅ WASM compressed: %s\n", getFileSize(wasmOut+".gz"))
	}
	return true
}

// CompileWASMFromSource builds the search engine WASM from Go source.
// This is used for developer convenience during development.
func CompileWASMFromSource(ctx context.Context, srcPath string, destPath string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go compiler not found in PATH")
	}

	absSrc, _ := filepath.Abs(srcPath)
	absDest, _ := filepath.Abs(destPath)

	fmt.Printf("🚀 Rebuilding Search WASM from source (%s)... \n", srcPath)

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(absDest), 0755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", absDest, absSrc)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile WASM: %w", err)
	}

	fmt.Println("✅ Search WASM rebuilt successfully.")
	return nil
}

// hashBytes computes BLAKE3 hash of byte slice (first 16 hex chars)
func hashBytes(data []byte) string {
	h := blake3.New()
	if _, err := h.Write(data); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// hashFile computes BLAKE3 hash of file contents
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func CompressGzip(src, dst string) error {
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
	_, copyErr := io.Copy(gw, in)
	closeErr := gw.Close()

	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func getFileSize(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%.2f KB", float64(fi.Size())/1024)
}
