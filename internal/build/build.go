package build

import (
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
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
func CheckWASM(outputDir string) bool {
	return CheckWASMFs(afero.NewOsFs(), outputDir)
}

func CheckWASMFs(fs afero.Fs, outputDir string) bool {
	return CheckWASMFsWithSource(fs, outputDir, nil)
}

func CheckWASMFsWithSource(fs afero.Fs, outputDir string, sourceWasm []byte) bool {
	wasmOut := filepath.Join(outputDir, "static/wasm/search.wasm")
	wasmBytes := searchWasm
	wasmHash := embeddedWasmHash
	if len(sourceWasm) > 0 {
		wasmBytes = sourceWasm
		wasmHash = hashBytes(sourceWasm)
	}

	if err := fs.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		fmt.Printf("⚠️ Failed to create WASM directory: %v\n", err)
	}

	// Check if deployed WASM matches embedded version
	if deployedHash, err := hashFileFs(fs, wasmOut); err == nil {
		if deployedHash == wasmHash {
			// Already up-to-date, skip write
			return false
		}
		fmt.Println("🔄 WASM updated, deploying new version...")
	} else {
		fmt.Println("🚀 Writing embedded Search WASM...")
	}

	// Write new WASM
	if err := afero.WriteFile(fs, wasmOut, wasmBytes, 0644); err != nil {
		fmt.Printf("❌ Failed to write WASM: %v\n", err)
		if !utils.TestingMode {
			os.Exit(1)
		}
		return false
	}

	// Compress WASM — stream once through gzip and brotli to avoid duplicate reads
	fmt.Println("📦 Compressing WASM...")
	if err := CompressDualFs(fs, wasmOut, wasmOut+".gz", wasmOut+".br"); err != nil {
		fmt.Printf("⚠️ Failed to compress WASM: %v\n", err)
	} else {
		fmt.Printf("✅ WASM gzipped: %s\n", getFileSizeFs(fs, wasmOut+".gz"))
		fmt.Printf("✅ WASM brotlied: %s\n", getFileSizeFs(fs, wasmOut+".br"))
	}
	return true
}

func DeployWASMFromFile(fs afero.Fs, outputDir, sourcePath string) bool {
	data, err := afero.ReadFile(fs, sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckWASMFs(fs, outputDir)
		}
		fmt.Printf("⚠️ Failed to read source WASM %s: %v\n", sourcePath, err)
		return CheckWASMFs(fs, outputDir)
	}
	return CheckWASMFsWithSource(fs, outputDir, data)
}

func RepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func RepoPath(parts ...string) string {
	all := append([]string{RepoRoot()}, parts...)
	return filepath.Join(all...)
}

// CompileWASMFromSource builds the search engine WASM from Go source.
// This is used for developer convenience during development.
func CompileWASMFromSource(ctx context.Context, srcPath string, destPath string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go compiler not found in PATH")
	}

	repoRoot := RepoRoot()
	absSrc := srcPath
	if !filepath.IsAbs(absSrc) {
		absSrc = filepath.Join(repoRoot, srcPath)
	}
	absDest := destPath
	if !filepath.IsAbs(absDest) {
		absDest = filepath.Join(repoRoot, destPath)
	}

	fmt.Printf("🚀 Rebuilding Search WASM from source (%s)... \n", srcPath)

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(absDest), 0755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", absDest, absSrc)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile WASM: %w", err)
	}

	fmt.Println("✅ Search WASM rebuilt successfully.")
	return nil
}

// hashBytes computes XXH3 hash of byte slice (first 16 hex chars)
func hashBytes(data []byte) string {
	h := xxh3.New()
	if _, err := h.Write(data); err != nil {
		return ""
	}
	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])[:16]
}

// hashFile computes XXH3 hash of file contents
func hashFile(path string) (string, error) {
	return hashFileFs(afero.NewOsFs(), path)
}

func hashFileFs(fs afero.Fs, path string) (string, error) {
    f, err := fs.Open(path)
    if err != nil {
        return "", err
    }
    defer func() { _ = f.Close() }()

    h := xxh3.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", err
    }
    sum := h.Sum128()
    b := sum.Bytes()
    return hex.EncodeToString(b[:])[:16], nil
}

func CompressGzip(src, dst string) error {
	return CompressGzipFs(afero.NewOsFs(), src, dst)
}

func CompressGzipFs(fs afero.Fs, src, dst string) error {
	in, err := fs.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := fs.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	gw, _ := gzip.NewWriterLevel(out, gzip.BestSpeed)
	_, copyErr := io.Copy(gw, in)
	closeErr := gw.Close()

	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func CompressBrotli(src, dst string) error {
	return CompressBrotliFs(afero.NewOsFs(), src, dst)
}

func CompressBrotliFs(fs afero.Fs, src, dst string) error {
	return CompressBrotliFsLevel(fs, src, dst, brotli.DefaultCompression)
}

// CompressBrotliFsLevel compresses a file with brotli at the given quality level (0-11).
func CompressBrotliFsLevel(fs afero.Fs, src, dst string, level int) error {
	in, err := fs.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := fs.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	bw := brotli.NewWriterLevel(out, level)
	_, copyErr := io.Copy(bw, in)
	closeErr := bw.Close()

	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// CompressDualFs streams the input once and writes both gzip (best speed) and brotli (level 4).
func CompressDualFs(fs afero.Fs, src, dstGzip, dstBrotli string) error {
	in, err := fs.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	gzOut, err := fs.Create(dstGzip)
	if err != nil {
		return err
	}
	defer func() { _ = gzOut.Close() }()

	brOut, err := fs.Create(dstBrotli)
	if err != nil {
		return err
	}
	defer func() { _ = brOut.Close() }()

	gzW, _ := gzip.NewWriterLevel(gzOut, gzip.BestSpeed)
	brW := brotli.NewWriterLevel(brOut, 4)

	tee := io.TeeReader(in, gzW)
	_, copyErr := io.Copy(brW, tee)
	brCloseErr := brW.Close()
	gzCloseErr := gzW.Close()

	if copyErr != nil {
		return copyErr
	}
	if brCloseErr != nil {
		return brCloseErr
	}
	return gzCloseErr
}

func getFileSize(path string) string {
	return getFileSizeFs(afero.NewOsFs(), path)
}

func getFileSizeFs(fs afero.Fs, path string) string {
	fi, err := fs.Stat(path)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%.2f KB", float64(fi.Size())/1024)
}
