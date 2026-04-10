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
func CheckWASMFs(sourceFs afero.Fs, sink fspkg.ArtifactSink, cacheDir string) bool {
	return CheckWASMFsWithSource(CheckWASMOptions{
		Fs:       sourceFs,
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
	wasmRelativePath := "static/wasm/search.wasm"
	brotliRelativePath := wasmRelativePath + ".br"
	wasmOutputPath := filepath.Join(outputDir, wasmRelativePath)
	brotliOutputPath := filepath.Join(outputDir, brotliRelativePath)
	return wasmRelativePath, brotliRelativePath, wasmOutputPath, brotliOutputPath
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

func ensureWasmDir(sink fspkg.ArtifactSink, wasmRelativePath string) {
	if err := sink.MkdirAll(filepath.Dir(wasmRelativePath)); err != nil {
		slog.Warn("Failed to create WASM directory", "error", err)
	}
}

func hasDeployedWasm(sourceFs afero.Fs, wasmOutputPath, brotliOutputPath, wasmHash string) bool {
	brotliExists, _ := afero.Exists(sourceFs, brotliOutputPath)
	if deployedHash, err := hashFileFs(sourceFs, wasmOutputPath); err == nil && brotliExists {
		if deployedHash == wasmHash {
			return true
		}
		slog.Info("WASM updated, deploying new version...")
	}
	return false
}

func tryDeployFromCache(sink fspkg.ArtifactSink, cacheDir, wasmHash string, wasmBytes []byte, wasmRelativePath, brotliRelativePath string) bool {
	if cacheDir == "" {
		return false
	}
	cachePath := filepath.Join(cacheDir, "wasm", wasmHash+".br")
	cachedBrotli, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}
	slog.Info("Using cached Search WASM...")
	_ = sink.WriteFile(wasmRelativePath, wasmBytes)
	_ = sink.WriteFile(brotliRelativePath, cachedBrotli)
	return true
}

func prepareEmbeddedWasm(wasmBrotliBytes []byte) ([]byte, []byte, bool) {
	wasmBytes, err := decompressBrotli(wasmBrotliBytes)
	if err != nil {
		slog.Error("Failed to decompress embedded WASM", "error", err)
		return nil, nil, false
	}
	return wasmBytes, wasmBrotliBytes, true
}

func prepareSourceWasm(wasmBytes []byte, compressionLevel int, cacheDir, wasmHash string) []byte {
	slog.Info("Compressing WASM...")
	var buffer bytes.Buffer
	brotliWriter := brotli.NewWriterLevel(&buffer, compressionLevel)
	_, _ = brotliWriter.Write(wasmBytes)
	if err := brotliWriter.Close(); err != nil {
		slog.Warn("Failed to close Brotli writer", "error", err)
	}
	wasmBrotliBytes := buffer.Bytes()

	if cacheDir != "" {
		cacheDirFull := filepath.Join(cacheDir, "wasm")
		_ = os.MkdirAll(cacheDirFull, wasmCacheDirMode)
		_ = os.WriteFile(filepath.Join(cacheDirFull, wasmHash+".br"), wasmBrotliBytes, wasmCacheFileMode)
	}

	return wasmBrotliBytes
}

func writeWasmOutputs(sink fspkg.ArtifactSink, wasmRelativePath, brotliRelativePath string, wasmBytes, wasmBrotliBytes []byte) bool {
	if err := sink.WriteFile(wasmRelativePath, wasmBytes); err != nil {
		slog.Error("Failed to write WASM", "error", err)
		return false
	}
	if err := sink.WriteFile(brotliRelativePath, wasmBrotliBytes); err != nil {
		slog.Error("Failed to write WASM.br", "error", err)
		return false
	}
	slog.Info("WASM deployed",
		"uncompressed", formatSize(len(wasmBytes)),
		"compressed", formatSize(len(wasmBrotliBytes)))
	return true
}

// CheckWASMFsWithSource checks and deploys WASM using a provided source payload.
func CheckWASMFsWithSource(options CheckWASMOptions) bool {
	sourceFs := options.Fs
	sink := options.Sink
	cacheDir := options.CacheDir
	compressionLevel := resolveCompressionLevel(options.CompressionLevel)

	outputDir := sink.GetOutputDir()
	wasmRelativePath, brotliRelativePath, wasmOutputPath, brotliOutputPath := resolveWasmPaths(outputDir)

	wasmBytes, wasmHash, wasmBrotliBytes, ok := loadWasmSource(options.SourceWasm)
	if !ok {
		return false
	}
	isEmbedded := len(options.SourceWasm) == 0

	ensureWasmDir(sink, wasmRelativePath)

	if hasDeployedWasm(sourceFs, wasmOutputPath, brotliOutputPath, wasmHash) {
		return false
	}
	if !isEmbedded && tryDeployFromCache(sink, cacheDir, wasmHash, wasmBytes, wasmRelativePath, brotliRelativePath) {
		return true
	}
	slog.Info("Deploying Search WASM...")

	if isEmbedded {
		wasmBytes, wasmBrotliBytes, ok = prepareEmbeddedWasm(wasmBrotliBytes)
		if !ok {
			return false
		}
	} else {
		wasmBrotliBytes = prepareSourceWasm(wasmBytes, compressionLevel, cacheDir, wasmHash)
	}

	return writeWasmOutputs(sink, wasmRelativePath, brotliRelativePath, wasmBytes, wasmBrotliBytes)
}

// DeployWASMFromFile deploys a WASM binary from a file path.
func DeployWASMFromFile(sourceFs afero.Fs, sink fspkg.ArtifactSink, cacheDir, sourcePath string) bool {
	return DeployWASMFromFileWithLevel(DeployWASMOptions{
		Fs:         sourceFs,
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
func DeployWASMFromFileWithLevel(options DeployWASMOptions) bool {
	data, err := afero.ReadFile(options.Fs, options.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CheckWASMFs(options.Fs, options.Sink, options.CacheDir)
		}
		slog.Warn("Failed to read source WASM", "path", options.SourcePath, "error", err)
		return CheckWASMFs(options.Fs, options.Sink, options.CacheDir)
	}
	return CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               options.Fs,
		Sink:             options.Sink,
		CacheDir:         options.CacheDir,
		SourceWasm:       data,
		CompressionLevel: options.Level,
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
	hasher := xxh3.New()
	if _, err := hasher.Write(data); err != nil {
		return ""
	}
	sum := hasher.Sum128()
	hashBytes := sum.Bytes()
	return hex.EncodeToString(hashBytes[:])[:wasmHashHexLength]
}

func hashFileFs(sourceFs afero.Fs, path string) (string, error) {
	file, err := sourceFs.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := xxh3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	sum := hasher.Sum128()
	hashBytes := sum.Bytes()
	return hex.EncodeToString(hashBytes[:])[:wasmHashHexLength], nil
}
