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

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

//go:embed wasm/search.wasm.br
var searchWasmBr []byte

// embeddedWasmHash is the hash of the raw (decompressed) embedded WASM.
// It mirrors SearchWasmHash to keep legacy tests stable.
var embeddedWasmHash = SearchWasmHash

// wasmInitErr captures initialization errors for embedded WASM.
var wasmInitErr error

// CheckWASM ensures the search engine WASM is present and up-to-date.
// Uses hash comparison to avoid unnecessary writes when WASM hasn't changed.
func CheckWASM(sink fspkg.ArtifactSink, cacheDir string) bool {
	return CheckWASMFs(afero.NewOsFs(), sink, cacheDir)
}

// CheckWASMFs verifies the deployed WASM using the provided filesystem.
func CheckWASMFs(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir string) bool {
	return CheckWASMFsWithSource(CheckWASMOptions{
		Fs:       fs,
		Sink:     sink,
		CacheDir: cacheDir,
	})
}

// CheckWASMOptions configures WASM deployment checks.
type CheckWASMOptions struct {
	Fs               afero.Fs
	Sink             fspkg.ArtifactSink
	CacheDir         string
	SourceWasm       []byte
	CompressionLevel int
}

// CheckWASMFsWithSource checks and deploys WASM using a provided source payload.
func CheckWASMFsWithSource(opts CheckWASMOptions) bool {
	fs := opts.Fs
	sink := opts.Sink
	cacheDir := opts.CacheDir
	sourceWasm := opts.SourceWasm
	compressionLevel := opts.CompressionLevel

	// Default compression level is 4 (balanced)
	if compressionLevel == 0 {
		compressionLevel = 4
	}

	wasmRelPath := "static/wasm/search.wasm"
	brRelPath := wasmRelPath + ".br"

	outputDir := sink.GetOutputDir()
	wasmOut := filepath.Join(outputDir, wasmRelPath)
	brOut := filepath.Join(outputDir, brRelPath)

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
		wasmHash = SearchWasmHash
		wasmBrBytes = searchWasmBr
	} else {
		wasmBytes = sourceWasm
		wasmHash = hashBytes(sourceWasm)
	}

	if err := sink.MkdirAll(filepath.Dir(wasmRelPath)); err != nil {
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
				_ = sink.WriteFile(wasmRelPath, wasmBytes)
				_ = sink.WriteFile(brRelPath, cachedBr)
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
		bw := brotli.NewWriterLevel(&buf, compressionLevel)
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

	if err := sink.WriteFile(wasmRelPath, wasmBytes); err != nil {
		slog.Error("Failed to write WASM", "error", err)
		return false
	}
	if err := sink.WriteFile(brRelPath, wasmBrBytes); err != nil {
		slog.Error("Failed to write WASM.br", "error", err)
		return false
	}

	slog.Info("WASM deployed",
		"uncompressed", formatSize(len(wasmBytes)),
		"compressed", formatSize(len(wasmBrBytes)))

	return true
}

// DeployWASMFromFile deploys a WASM binary from a file path.
func DeployWASMFromFile(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir, sourcePath string) bool {
	return DeployWASMFromFileWithLevel(DeployWASMOptions{
		Fs:         fs,
		Sink:       sink,
		CacheDir:   cacheDir,
		SourcePath: sourcePath,
		Level:      4,
	})
}

// DeployWASMOptions configures WASM deployment from a source file.
type DeployWASMOptions struct {
	Fs         afero.Fs
	Sink       fspkg.ArtifactSink
	CacheDir   string
	SourcePath string
	Level      int
}

// DeployWASMFromFileWithLevel deploys a WASM file with a specific compression level.
func DeployWASMFromFileWithLevel(opts DeployWASMOptions) bool {
	data, err := afero.ReadFile(opts.Fs, opts.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckWASMFs(opts.Fs, opts.Sink, opts.CacheDir)
		}
		slog.Warn("Failed to read source WASM", "path", opts.SourcePath, "error", err)
		return CheckWASMFs(opts.Fs, opts.Sink, opts.CacheDir)
	}
	return CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               opts.Fs,
		Sink:             opts.Sink,
		CacheDir:         opts.CacheDir,
		SourceWasm:       data,
		CompressionLevel: opts.Level,
	})
}

// CompileWASMFromSource builds the search engine WASM from Go source.
// This is used for developer convenience during development.
func CompileWASMFromSource(ctx context.Context, srcPath string, destPath string, repoRoot string) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go compiler not found in PATH: %w", err)
	}

	repoRoot = fspkg.NormalizePath(repoRoot)
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
