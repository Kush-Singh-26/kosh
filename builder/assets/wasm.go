package assets

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

	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/utils"
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
			slog.Error("WASM initialization failed", "error", wasmInitErr)
			return false
		}
		wasmHash = embeddedWasmHash
		wasmBrBytes = searchWasmBr
	} else {
		wasmBytes = sourceWasm
		wasmHash = hashBytes(sourceWasm)
	}

	if err := fs.MkdirAll(filepath.Dir(wasmOut), 0755); err != nil {
		slog.Warn("Failed to create WASM directory", "error", err)
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

func DeployWASMFromFile(fs afero.Fs, outputDir, cacheDir, sourcePath string) bool {
	data, err := afero.ReadFile(fs, sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckWASMFs(fs, outputDir, cacheDir)
		}
		slog.Warn("Failed to read source WASM", "path", sourcePath, "error", err)
		return CheckWASMFs(fs, outputDir, cacheDir)
	}
	return CheckWASMFsWithSource(fs, outputDir, cacheDir, data)
}

// CompileWASMFromSource builds the search engine WASM from Go source.
// This is used for developer convenience during development.
func CompileWASMFromSource(ctx context.Context, srcPath string, destPath string) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go compiler not found in PATH: %w", err)
	}

	repoRoot := utils.RepoRoot()
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

	cmd := exec.CommandContext(ctx, goPath, "build", "-o", absDest, absSrc)
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

func decompressBrotli(data []byte) ([]byte, error) {
	br := brotli.NewReader(bytes.NewReader(data))
	return io.ReadAll(br)
}

func formatSize(size int) string {
	return fmt.Sprintf("%.2f KB", float64(size)/1024)
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
