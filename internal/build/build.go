package build

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

//go:embed wasm/search.wasm.br
var searchWasmBr []byte

// embeddedWasmHash caches the hash of the raw (decompressed) embedded WASM
var embeddedWasmHash string
var wasmInitErr error

func init() {
	raw, err := decompressBrotli(searchWasmBr)
	if err != nil {
		// This should never happen if the build process is correct
		// Store error for lazy handling instead of panicking
		wasmInitErr = fmt.Errorf("failed to decompress embedded WASM: %w", err)
		return
	}
	embeddedWasmHash = hashBytes(raw)
}

// CheckWASM ensures the search engine WASM is present and up-to-date.
// Uses hash comparison to avoid unnecessary writes when WASM hasn't changed.
func CheckWASM(outputDir string, cacheDir string) bool {
	return CheckWASMFs(afero.NewOsFs(), outputDir, cacheDir)
}

func CheckWASMFs(fs afero.Fs, outputDir string, cacheDir string) bool {
	return CheckWASMFsWithSource(fs, outputDir, cacheDir, nil)
}

func CheckWASMFsWithSource(fs afero.Fs, outputDir string, cacheDir string, sourceWasm []byte) bool {
	wasmOut := filepath.Join(outputDir, "static/wasm/search.wasm")
	brOut := wasmOut + ".br"

	var wasmBytes []byte
	var wasmHash string
	var wasmBrBytes []byte
	isEmbedded := len(sourceWasm) == 0

	if isEmbedded {
		// Check for initialization error
		if wasmInitErr != nil {
			fmt.Printf("❌ WASM initialization failed: %v\n", wasmInitErr)
			return false
		}
		wasmHash = embeddedWasmHash
		wasmBrBytes = searchWasmBr
	} else {
		wasmBytes = sourceWasm
		wasmHash = hashBytes(sourceWasm)
	}

	if err := fs.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		fmt.Printf("⚠️ Failed to create WASM directory: %v\n", err)
	}

	// Check if deployed WASM matches current version
	brExists, _ := afero.Exists(fs, brOut)
	if deployedHash, err := hashFileFs(fs, wasmOut); err == nil && brExists {
		if deployedHash == wasmHash {
			return false
		}
		slog.Info("WASM updated, deploying new version...")
	} else {
		// Check persistent cache for non-embedded source
		if !isEmbedded && cacheDir != "" {
			cachePath := filepath.Join(cacheDir, "wasm", wasmHash+".br")
			if cachedBr, err := os.ReadFile(cachePath); err == nil {
				slog.Info("Using cached Search WASM...")
				_ = afero.WriteFile(fs, wasmOut, wasmBytes, 0644)
				_ = afero.WriteFile(fs, brOut, cachedBr, 0644)
				return true
			}
		}
		slog.Info("Deploying Search WASM...")
	}

	// Deploy WASM and Brotli
	if isEmbedded {
		// For embedded, we have .br, need to decompress for .wasm
		var err error
		wasmBytes, err = decompressBrotli(wasmBrBytes)
		if err != nil {
			slog.Error("Failed to decompress embedded WASM", "error", err)
			return false
		}
	} else {
		// For source, we have .wasm, need to compress for .br
		slog.Info("Compressing WASM...")
		var buf bytes.Buffer
		bw := brotli.NewWriterLevel(&buf, 4)
		_, _ = bw.Write(wasmBytes)
		if err := bw.Close(); err != nil {
			slog.Warn("Failed to close Brotli writer", "error", err)
		}
		wasmBrBytes = buf.Bytes()

		// Save to persistent cache
		if cacheDir != "" {
			cacheDirFull := filepath.Join(cacheDir, "wasm")
			_ = os.MkdirAll(cacheDirFull, 0755)
			_ = os.WriteFile(filepath.Join(cacheDirFull, wasmHash+".br"), wasmBrBytes, 0644)
		}
	}

	if err := afero.WriteFile(fs, wasmOut, wasmBytes, 0644); err != nil {
		slog.Error("Failed to write WASM", "error", err)
		return false
	}
	if err := afero.WriteFile(fs, brOut, wasmBrBytes, 0644); err != nil {
		slog.Error("Failed to write WASM.br", "error", err)
		return false
	}

	slog.Info("WASM deployed",
		"uncompressed", formatSize(len(wasmBytes)),
		"compressed", formatSize(len(wasmBrBytes)))

	return true
}

func decompressBrotli(data []byte) ([]byte, error) {
	br := brotli.NewReader(bytes.NewReader(data))
	return io.ReadAll(br)
}

func formatSize(size int) string {
	return fmt.Sprintf("%.2f KB", float64(size)/1024)
}

func DeployWASMFromFile(fs afero.Fs, outputDir, cacheDir, sourcePath string) bool {
	data, err := afero.ReadFile(fs, sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckWASMFs(fs, outputDir, cacheDir)
		}
		fmt.Printf("⚠️ Failed to read source WASM %s: %v\n", sourcePath, err)
		return CheckWASMFs(fs, outputDir, cacheDir)
	}
	return CheckWASMFsWithSource(fs, outputDir, cacheDir, data)
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

	slog.Info("Rebuilding Search WASM from source", "path", srcPath)

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(absDest), 0755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", absDest, absSrc)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile WASM: %w", err)
	}

	slog.Info("Search WASM rebuilt successfully")
	return nil
}

func CompileKaTeXBytecode(ctx context.Context, bcPath string) error {
	repoRoot := RepoRoot()
	absDest := bcPath
	if !filepath.IsAbs(absDest) {
		absDest = filepath.Join(repoRoot, bcPath)
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "compile-katex", "main.go")
	cmd := exec.CommandContext(ctx, "go", "run", scriptPath, absDest)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
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
