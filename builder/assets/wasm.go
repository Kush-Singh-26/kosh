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

const (
	defaultCompressionLevel = 4
	wasmCacheDirMode        = 0755
	wasmCacheFileMode       = 0644
	wasmHashHexLength       = 16
	kilobyteSize            = 1024
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

func resolveCompressionLevel(level int) int {
	if level == 0 {
		return defaultCompressionLevel
	}
	return level
}

func resolveWasmPaths(outputDir string) (string, string, string, string) {
	wasmRelPath := "static/wasm/search.wasm"
	brRelPath := wasmRelPath + ".br"
	wasmOut := filepath.Join(outputDir, wasmRelPath)
	brOut := filepath.Join(outputDir, brRelPath)
	return wasmRelPath, brRelPath, wasmOut, brOut
}

func loadWasmSource(sourceWasm []byte) ([]byte, string, []byte, bool) {
	if len(sourceWasm) == 0 {
		if wasmInitErr != nil {
			slog.Error("WASM initialization failed", "error", wasmInitErr)
			return nil, "", nil, false
		}
		return nil, SearchWasmHash, searchWasmBr, true
	}
	return sourceWasm, hashBytes(sourceWasm), nil, true
}

func ensureWasmDir(sink fspkg.ArtifactSink, wasmRelPath string) {
	if err := sink.MkdirAll(filepath.Dir(wasmRelPath)); err != nil {
		slog.Warn("Failed to create WASM directory", "error", err)
	}
}

func hasDeployedWasm(fs afero.Fs, wasmOut, brOut, wasmHash string) bool {
	brExists, _ := afero.Exists(fs, brOut)
	if deployedHash, err := hashFileFs(fs, wasmOut); err == nil && brExists {
		if deployedHash == wasmHash {
			return true
		}
		slog.Info("WASM updated, deploying new version...")
	}
	return false
}

func tryDeployFromCache(sink fspkg.ArtifactSink, cacheDir, wasmHash string, wasmBytes []byte, wasmRelPath, brRelPath string) bool {
	if cacheDir == "" {
		return false
	}
	cachePath := filepath.Join(cacheDir, "wasm", wasmHash+".br")
	cachedBr, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}
	slog.Info("Using cached Search WASM...")
	_ = sink.WriteFile(wasmRelPath, wasmBytes)
	_ = sink.WriteFile(brRelPath, cachedBr)
	return true
}

func prepareEmbeddedWasm(wasmBrBytes []byte) ([]byte, []byte, bool) {
	wasmBytes, err := decompressBrotli(wasmBrBytes)
	if err != nil {
		slog.Error("Failed to decompress embedded WASM", "error", err)
		return nil, nil, false
	}
	return wasmBytes, wasmBrBytes, true
}

func prepareSourceWasm(wasmBytes []byte, compressionLevel int, cacheDir, wasmHash string) []byte {
	slog.Info("Compressing WASM...")
	var buf bytes.Buffer
	bw := brotli.NewWriterLevel(&buf, compressionLevel)
	_, _ = bw.Write(wasmBytes)
	if err := bw.Close(); err != nil {
		slog.Warn("Failed to close Brotli writer", "error", err)
	}
	wasmBrBytes := buf.Bytes()

	if cacheDir != "" {
		cacheDirFull := filepath.Join(cacheDir, "wasm")
		_ = os.MkdirAll(cacheDirFull, wasmCacheDirMode)
		_ = os.WriteFile(filepath.Join(cacheDirFull, wasmHash+".br"), wasmBrBytes, wasmCacheFileMode)
	}

	return wasmBrBytes
}

func writeWasmOutputs(sink fspkg.ArtifactSink, wasmRelPath, brRelPath string, wasmBytes, wasmBrBytes []byte) bool {
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

// CheckWASMFsWithSource checks and deploys WASM using a provided source payload.
func CheckWASMFsWithSource(opts CheckWASMOptions) bool {
	fs := opts.Fs
	sink := opts.Sink
	cacheDir := opts.CacheDir
	compressionLevel := resolveCompressionLevel(opts.CompressionLevel)

	outputDir := sink.GetOutputDir()
	wasmRelPath, brRelPath, wasmOut, brOut := resolveWasmPaths(outputDir)

	wasmBytes, wasmHash, wasmBrBytes, ok := loadWasmSource(opts.SourceWasm)
	if !ok {
		return false
	}
	isEmbedded := len(opts.SourceWasm) == 0

	ensureWasmDir(sink, wasmRelPath)

	if hasDeployedWasm(fs, wasmOut, brOut, wasmHash) {
		return false
	}
	if !isEmbedded && tryDeployFromCache(sink, cacheDir, wasmHash, wasmBytes, wasmRelPath, brRelPath) {
		return true
	}
	slog.Info("Deploying Search WASM...")

	if isEmbedded {
		wasmBytes, wasmBrBytes, ok = prepareEmbeddedWasm(wasmBrBytes)
		if !ok {
			return false
		}
	} else {
		wasmBrBytes = prepareSourceWasm(wasmBytes, compressionLevel, cacheDir, wasmHash)
	}

	return writeWasmOutputs(sink, wasmRelPath, brRelPath, wasmBytes, wasmBrBytes)
}

// DeployWASMFromFile deploys a WASM binary from a file path.
func DeployWASMFromFile(fs afero.Fs, sink fspkg.ArtifactSink, cacheDir, sourcePath string) bool {
	return DeployWASMFromFileWithLevel(DeployWASMOptions{
		Fs:         fs,
		Sink:       sink,
		CacheDir:   cacheDir,
		SourcePath: sourcePath,
		Level:      defaultCompressionLevel,
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
	if err := os.MkdirAll(filepath.Dir(absDest), wasmCacheDirMode); err != nil {
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
	return fmt.Sprintf("%.2f KB", float64(size)/kilobyteSize)
}

// hashBytes computes XXH3 hash of byte slice (first 16 hex chars)
func hashBytes(data []byte) string {
	h := xxh3.New()
	if _, err := h.Write(data); err != nil {
		return ""
	}
	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])[:wasmHashHexLength]
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
	return hex.EncodeToString(b[:])[:wasmHashHexLength], nil
}
