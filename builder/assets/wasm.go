//go:build !wasm

package assets

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

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

//go:embed wasm/wasm_exec.js
var wasmExecJs []byte

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

func prepareEmbeddedWasm(wasmBrotliBytes []byte) ([]byte, []byte, bool) {
	wasmBytes, err := decompressBrotli(wasmBrotliBytes)
	if err != nil {
		slog.Error("Failed to decompress embedded WASM", "error", err)
		return nil, nil, false
	}
	return wasmBytes, wasmBrotliBytes, true
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

var (
	wasmExecJsDeployed bool
	wasmExecJsMu       sync.Mutex
)

// DeployWasmExec deploys the Go WASM runtime stub to static/js/
// Returns true if deployment was successful or already present
func DeployWasmExec(sink fspkg.ArtifactSink) bool {
	wasmExecJsMu.Lock()
	defer wasmExecJsMu.Unlock()

	if wasmExecJsDeployed {
		return true
	}

	jsPath := "static/js/wasm_exec.js"

	// Check if already deployed by comparing hash
	if _, err := sink.Stat(jsPath); err == nil {
		wasmExecJsDeployed = true
		return true
	}

	if err := sink.WriteFile(jsPath, wasmExecJs); err != nil {
		slog.Error("Failed to deploy wasm_exec.js", "error", err)
		return false
	}

	slog.Info("WASM exec deployed", "size", formatSize(len(wasmExecJs)))
	wasmExecJsDeployed = true
	return true
}

// ResetWasmExecDeployment resets the deployment flag for testing
func ResetWasmExecDeployment() {
	wasmExecJsMu.Lock()
	defer wasmExecJsMu.Unlock()
	wasmExecJsDeployed = false
}

// ResetWasmExecForBuild resets the deployment flag at the start of each build.
// This ensures wasm_exec.js is deployed even in incremental dev builds.
func ResetWasmExecForBuild() {
	wasmExecJsMu.Lock()
	defer wasmExecJsMu.Unlock()
	wasmExecJsDeployed = false
}

// CheckWASMFsWithSource checks and deploys WASM using a provided source payload.
func CheckWASMFsWithSource(options CheckWASMOptions) bool {
	sourceFs := options.Fs
	sink := options.Sink

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

	if isEmbedded {
		wasmBytes, wasmBrotliBytes, ok = prepareEmbeddedWasm(wasmBrotliBytes)
		if !ok {
			return false
		}
	} else {
		// This path is used when a sourceWasm payload is provided (e.g., from tests)
		slog.Info("Compressing WASM payload...")
		var buffer bytes.Buffer
		brotliWriter := brotli.NewWriterLevel(&buffer, resolveCompressionLevel(options.CompressionLevel))
		_, _ = brotliWriter.Write(wasmBytes)
		if err := brotliWriter.Close(); err != nil {
			slog.Warn("Failed to close Brotli writer", "error", err)
		}
		wasmBrotliBytes = buffer.Bytes()
	}

	return writeWasmOutputs(sink, wasmRelativePath, brotliRelativePath, wasmBytes, wasmBrotliBytes)
}

// CalculateSearchSourceHash computes the XXH3 hash of the search engine source code.
func CalculateSearchSourceHash(repoRoot string) (string, error) {
	searchPaths := []string{
		fspkg.NormalizePath(filepath.Join(repoRoot, "cmd", "search")),
		fspkg.NormalizePath(filepath.Join(repoRoot, "builder", "search")),
		fspkg.NormalizePath(filepath.Join(repoRoot, "builder", "models")),
	}

	hasher := xxh3.New()
	var files []string

	for _, searchPath := range searchPaths {
		_ = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Ext(path) == ".go" {
				files = append(files, path)
			}
			return nil
		})
	}

	if len(files) == 0 {
		return "", os.ErrNotExist
	}

	sort.Strings(files)

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		_, _ = io.Copy(hasher, f)
		_ = f.Close()
	}

	sum := hasher.Sum128()
	hashBytes := sum.Bytes()
	// Return 16 chars hex to match SearchWasmHash format
	return hex.EncodeToString(hashBytes[:8]), nil
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
