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

	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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

// CheckWASM ensures the search engine WASM is present and up-to-date.
// Uses hash comparison to avoid unnecessary writes when WASM hasn't changed.
func CheckWASM(sink fspkg.ArtifactSink, cacheDir string) (bool, error) {
	return CheckWASMFs(afero.NewOsFs(), sink, cacheDir)
}

// CheckWASMFs verifies the deployed WASM using the provided filesystem.
func CheckWASMFs(sourceFs afero.Fs, sink fspkg.ArtifactSink, cacheDir string) (bool, error) {
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

func loadWasmSource(sourceWasm []byte) ([]byte, string, []byte, error) {
	if len(sourceWasm) == 0 {
		return nil, SearchWasmHash, searchWasmBr, nil
	}
	return sourceWasm, hashBytes(sourceWasm), nil, nil
}

func ensureWasmDir(sink fspkg.ArtifactSink, wasmRelativePath string) error {
	if err := sink.MkdirAll(filepath.Dir(wasmRelativePath)); err != nil {
		return fmt.Errorf("failed to create WASM directory: %w", err)
	}
	return nil
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

func prepareEmbeddedWasm(wasmBrotliBytes []byte) ([]byte, []byte, error) {
	wasmBytes, err := decompressBrotli(wasmBrotliBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decompress embedded WASM: %w", err)
	}
	return wasmBytes, wasmBrotliBytes, nil
}

func writeWasmOutputs(sink fspkg.ArtifactSink, wasmRelativePath, brotliRelativePath string, wasmBytes, wasmBrotliBytes []byte) error {
	if err := sink.WriteFile(wasmRelativePath, wasmBytes); err != nil {
		return fmt.Errorf("failed to write WASM: %w", err)
	}
	if err := sink.WriteFile(brotliRelativePath, wasmBrotliBytes); err != nil {
		return fmt.Errorf("failed to write WASM.br: %w", err)
	}
	slog.Info("WASM deployed",
		"uncompressed", formatSize(len(wasmBytes)),
		"compressed", formatSize(len(wasmBrotliBytes)))
	return nil
}

var (
	wasmExecJsDeployed bool
	wasmExecJsMu       sync.Mutex
)

// DeployWasmExec deploys the Go WASM runtime stub to static/js/
// Returns nil if deployment was successful or already present
func DeployWasmExec(sink fspkg.ArtifactSink) error {
	wasmExecJsMu.Lock()
	defer wasmExecJsMu.Unlock()

	if wasmExecJsDeployed {
		return nil
	}

	jsPath := "static/js/wasm_exec.js"

	// Check if already deployed by comparing hash
	if _, err := sink.Stat(jsPath); err == nil {
		wasmExecJsDeployed = true
		return nil
	}

	if err := sink.WriteFile(jsPath, wasmExecJs); err != nil {
		return fmt.Errorf("failed to deploy wasm_exec.js: %w", err)
	}

	slog.Info("WASM exec deployed", "size", formatSize(len(wasmExecJs)))
	wasmExecJsDeployed = true
	return nil
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
func CheckWASMFsWithSource(options CheckWASMOptions) (bool, error) {
	sourceFs := options.Fs
	sink := options.Sink

	outputDir := sink.GetOutputDir()
	wasmRelativePath, brotliRelativePath, wasmOutputPath, brotliOutputPath := resolveWasmPaths(outputDir)

	wasmBytes, wasmHash, wasmBrotliBytes, err := loadWasmSource(options.SourceWasm)
	if err != nil {
		return false, err
	}
	isEmbedded := len(options.SourceWasm) == 0

	if err := ensureWasmDir(sink, wasmRelativePath); err != nil {
		return false, err
	}

	if hasDeployedWasm(sourceFs, wasmOutputPath, brotliOutputPath, wasmHash) {
		return false, nil
	}

	if isEmbedded {
		var err error
		wasmBytes, wasmBrotliBytes, err = prepareEmbeddedWasm(wasmBrotliBytes)
		if err != nil {
			return false, err
		}
	} else {
		// This path is used when a sourceWasm payload is provided (e.g., from tests)
		slog.Info("Compressing WASM payload...")
		var buffer bytes.Buffer
		brotliWriter := brotli.NewWriterLevel(&buffer, resolveCompressionLevel(options.CompressionLevel))
		_, _ = brotliWriter.Write(wasmBytes)
		if err := brotliWriter.Close(); err != nil {
			return false, fmt.Errorf("failed to close Brotli writer: %w", err)
		}
		wasmBrotliBytes = buffer.Bytes()
	}

	return true, writeWasmOutputs(sink, wasmRelativePath, brotliRelativePath, wasmBytes, wasmBrotliBytes)
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
		func() {
			f, err := os.Open(file)
			if err != nil {
				return
			}
			defer func() { _ = f.Close() }()
			_, _ = io.Copy(hasher, f)
		}()
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
