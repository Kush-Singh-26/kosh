package assets

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

// fsSink wraps MemSink and mirrors writes to an afero.Fs so that
// hashFileFs checks (which read from afero) see the deployed files.
type fsSink struct {
	*testutil.MemSink
	fs afero.Fs
}

func newFsSink(outputDir string, fs afero.Fs) *fsSink {
	return &fsSink{MemSink: testutil.NewMemSinkWithDir(outputDir), fs: fs}
}

func (f *fsSink) WriteFile(path string, data []byte) error {
	absPath := filepath.Join(f.OutputDir, path)
	if err := f.fs.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	if err := afero.WriteFile(f.fs, absPath, data, 0644); err != nil {
		return err
	}
	return f.MemSink.WriteFile(path, data)
}

func (f *fsSink) MkdirAll(path string) error {
	absPath := filepath.Join(f.OutputDir, path)
	return f.fs.MkdirAll(absPath, 0755)
}

func (f *fsSink) Stat(path string) (os.FileInfo, error) {
	absPath := filepath.Join(f.OutputDir, path)
	return f.fs.Stat(absPath)
}

// dirSink writes directly to a real directory (for tests using afero.NewOsFs).
type dirSink struct {
	outputDir string
}

func newDirSink(dir string) *dirSink {
	return &dirSink{outputDir: dir}
}

func (d *dirSink) WriteFile(path string, data []byte) error {
	absPath := filepath.Join(d.outputDir, path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0644)
}

func (d *dirSink) WriteStream(path string, fn func(io.Writer) error) error {
	absPath := filepath.Join(d.outputDir, path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(absPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return fn(f)
}

func (d *dirSink) CopyFile(srcPath, destPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return d.WriteFile(destPath, data)
}

func (d *dirSink) MkdirAll(path string) error {
	return os.MkdirAll(filepath.Join(d.outputDir, path), 0755)
}

func (d *dirSink) Register(_ string)                {}
func (d *dirSink) GetWrittenFiles() map[string]bool { return nil }
func (d *dirSink) GetOutputDir() string             { return d.outputDir }
func (d *dirSink) SetMtime(path string, mtime time.Time) error {
	return os.Chtimes(filepath.Join(d.outputDir, path), mtime, mtime)
}

func (d *dirSink) Stat(path string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(d.outputDir, path))
}

// TestHashBytes tests the hashBytes function
func TestHashBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantLen int
	}{
		{
			name:    "empty bytes",
			input:   []byte{},
			wantLen: 16,
		},
		{
			name:    "simple content",
			input:   []byte("hello world"),
			wantLen: 16,
		},
		{
			name:    "binary content",
			input:   []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			wantLen: 16,
		},
		{
			name:    "large content",
			input:   bytes.Repeat([]byte("x"), 10000),
			wantLen: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashBytes(tt.input)
			if len(hash) != tt.wantLen {
				t.Errorf("hashBytes() returned hash of length %d, want %d", len(hash), tt.wantLen)
			}
		})
	}
}

// TestHashBytesDeterministic tests that hashBytes produces consistent results
func TestHashBytesDeterministic(t *testing.T) {
	input := []byte("test content for hashing")

	hash1 := hashBytes(input)
	hash2 := hashBytes(input)

	if hash1 != hash2 {
		t.Errorf("hashBytes() not deterministic: %s != %s", hash1, hash2)
	}
}

// TestHashBytesDifferentContent tests that different content produces different hashes
func TestHashBytesDifferentContent(t *testing.T) {
	hash1 := hashBytes([]byte("content A"))
	hash2 := hashBytes([]byte("content B"))

	if hash1 == hash2 {
		t.Error("Different content should produce different hashes")
	}
}

// TestDecompressBrotli tests brotli decompression
func TestDecompressBrotli(t *testing.T) {
	// Create test data
	original := []byte("test content for brotli compression and decompression")

	// Compress
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	_, _ = bw.Write(original)
	_ = bw.Close()
	compressed := buf.Bytes()

	// Decompress
	decompressed, err := decompressBrotli(compressed)
	if err != nil {
		t.Fatalf("decompressBrotli() error = %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("decompressBrotli() result mismatch: got %s, want %s", string(decompressed), string(original))
	}
}

// TestDecompressBrotliEmpty tests decompression of empty data
func TestDecompressBrotliEmpty(t *testing.T) {
	decompressed, err := decompressBrotli([]byte{})
	// Empty input produces EOF error - this is expected behavior
	if err == nil {
		t.Error("decompressBrotli() should return error on empty input")
	}
	// Should produce empty output
	if len(decompressed) != 0 {
		t.Errorf("decompressBrotli() expected empty output for empty input, got %d bytes", len(decompressed))
	}
}

// TestFormatSize tests the formatSize function
func TestFormatSize(t *testing.T) {
	tests := []struct {
		name string
		size int
		want string
	}{
		{
			name: "zero bytes",
			size: 0,
			want: "0.00 KB",
		},
		{
			name: "1 KB",
			size: 1024,
			want: "1.00 KB",
		},
		{
			name: "1.5 KB",
			size: 1536,
			want: "1.50 KB",
		},
		{
			name: "10 KB",
			size: 10240,
			want: "10.00 KB",
		},
		{
			name: "100 bytes",
			size: 100,
			want: "0.10 KB",
		},
		{
			name: "large file",
			size: 102400,
			want: "100.00 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.size)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %s, want %s", tt.size, got, tt.want)
			}
		})
	}
}

// TestCheckWASMFsWithEmbedded tests CheckWASMFs with embedded WASM
func TestCheckWASMFsWithEmbedded(t *testing.T) {
	fs := afero.NewMemMapFs()
	outputDir := "/output"
	cacheDir := "/cache"
	sink := newFsSink(outputDir, fs)

	// Test with embedded WASM (sourceWasm = nil)
	result, err := CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               fs,
		Sink:             sink,
		CacheDir:         cacheDir,
		SourceWasm:       nil,
		CompressionLevel: 0,
	})

	// Should deploy embedded WASM successfully
	if err != nil || !result {
		t.Errorf("CheckWASMFsWithSource() with embedded WASM failed: %v", err)
	}

	// Verify files were created
	wasmPath := filepath.Join(outputDir, "static/wasm/search.wasm")
	brPath := wasmPath + ".br"

	exists, _ := afero.Exists(fs, wasmPath)
	if !exists {
		t.Errorf("WASM file not created at %s", wasmPath)
	}

	exists, _ = afero.Exists(fs, brPath)
	if !exists {
		t.Errorf("Brotli WASM file not created at %s", brPath)
	}
}

// TestCheckWASMFsWithSourceWasm tests CheckWASMFs with provided source WASM
func TestCheckWASMFsWithSourceWasm(t *testing.T) {
	fs := afero.NewMemMapFs()
	outputDir := "/output"
	cacheDir := "/cache"
	sink := newFsSink(outputDir, fs)

	// Create test WASM data
	sourceWasm := []byte("test wasm content")

	result, err := CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               fs,
		Sink:             sink,
		CacheDir:         cacheDir,
		SourceWasm:       sourceWasm,
		CompressionLevel: 0,
	})

	if err != nil || !result {
		t.Errorf("CheckWASMFsWithSource() with source WASM failed: %v", err)
	}

	// Verify files were created
	wasmPath := filepath.Join(outputDir, "static/wasm/search.wasm")
	exists, _ := afero.Exists(fs, wasmPath)
	if !exists {
		t.Errorf("WASM file not created at %s", wasmPath)
	}
}

// TestCheckWASMFsNoChange tests that CheckWASMFs returns false when WASM hasn't changed
func TestCheckWASMFsNoChange(t *testing.T) {
	fs := afero.NewMemMapFs()
	outputDir := "/output"
	cacheDir := "/cache"
	sink := newFsSink(outputDir, fs)

	// First call - should deploy
	sourceWasm := []byte("test wasm content")
	result1, err := CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               fs,
		Sink:             sink,
		CacheDir:         cacheDir,
		SourceWasm:       sourceWasm,
		CompressionLevel: 0,
	})
	if err != nil || !result1 {
		t.Errorf("First CheckWASMFsWithSource() failed: %v", err)
	}

	// Second call with same content - should return false (no change)
	result2, err := CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               fs,
		Sink:             sink,
		CacheDir:         cacheDir,
		SourceWasm:       sourceWasm,
		CompressionLevel: 0,
	})
	if err != nil || result2 {
		t.Errorf("Second CheckWASMFsWithSource() should not have deployed: err=%v, result=%v", err, result2)
	}
}

func TestCheckWASMFsDirectoryCreation(t *testing.T) {
	fs := afero.NewMemMapFs()
	outputDir := "/deep/nested/output/path"
	cacheDir := "/cache"
	sink := newFsSink(outputDir, fs)

	sourceWasm := []byte("test wasm content")
	result, err := CheckWASMFsWithSource(CheckWASMOptions{
		Fs:               fs,
		Sink:             sink,
		CacheDir:         cacheDir,
		SourceWasm:       sourceWasm,
		CompressionLevel: 0,
	})

	if err != nil || !result {
		t.Errorf("CheckWASMFsWithSource() failed: %v", err)
	}

	// Verify directory was created
	wasmPath := filepath.Join(outputDir, "static/wasm/search.wasm")
	exists, _ := afero.Exists(fs, wasmPath)
	if !exists {
		t.Errorf("WASM file not created at %s", wasmPath)
	}
}

// TestHashFileFs tests the hashFileFs function
func TestHashFileFs(t *testing.T) {
	fs := afero.NewMemMapFs()
	testPath := "/test/file.txt"
	content := []byte("test content for file hashing")

	_ = afero.WriteFile(fs, testPath, content, 0644)

	hash, err := hashFileFs(fs, testPath)
	if err != nil {
		t.Fatalf("hashFileFs() error = %v", err)
	}

	if len(hash) != 16 {
		t.Errorf("hashFileFs() returned hash of length %d, want 16", len(hash))
	}
}

// TestHashFileFsDeterministic tests that hashFileFs produces consistent results
func TestHashFileFsDeterministic(t *testing.T) {
	fs := afero.NewMemMapFs()
	testPath := "/test/file.txt"
	content := []byte("test content for deterministic hashing")

	_ = afero.WriteFile(fs, testPath, content, 0644)

	hash1, _ := hashFileFs(fs, testPath)
	hash2, _ := hashFileFs(fs, testPath)

	if hash1 != hash2 {
		t.Errorf("hashFileFs() not deterministic: %s != %s", hash1, hash2)
	}
}

// TestHashFileFsNotFound tests hashFileFs with non-existent file
func TestHashFileFsNotFound(t *testing.T) {
	fs := afero.NewMemMapFs()
	testPath := "/nonexistent/file.txt"

	_, err := hashFileFs(fs, testPath)
	if err == nil {
		t.Error("hashFileFs() should return error for non-existent file")
	}
}

// TestHashFileFsDifferentFiles tests that different files produce different hashes
func TestHashFileFsDifferentFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	path1 := "/test/file1.txt"
	path2 := "/test/file2.txt"

	_ = afero.WriteFile(fs, path1, []byte("content A"), 0644)
	_ = afero.WriteFile(fs, path2, []byte("content B"), 0644)

	hash1, _ := hashFileFs(fs, path1)
	hash2, _ := hashFileFs(fs, path2)

	if hash1 == hash2 {
		t.Error("Different files should produce different hashes")
	}
}

// TestEmbeddedWasmHash tests that embeddedWasmHash is initialized
func TestEmbeddedWasmHash(t *testing.T) {
	if embeddedWasmHash == "" {
		t.Error("embeddedWasmHash should be initialized during init()")
	}

	if len(embeddedWasmHash) != 16 {
		t.Errorf("embeddedWasmHash should be 16 characters, got %d", len(embeddedWasmHash))
	}
}

// TestWasmInitErr tests that wasmInitErr is nil (embedded WASM should be valid)
func TestWasmInitErr(t *testing.T) {
	if wasmInitErr != nil {
		t.Errorf("wasmInitErr should be nil, got: %v", wasmInitErr)
	}
}

// TestCheckWASM tests the public CheckWASM function
func TestCheckWASM(t *testing.T) {
	// Create temp directory for real filesystem writes
	tmpDir := t.TempDir()
	cacheDir := ""
	sink := newDirSink(tmpDir)

	result, err := CheckWASM(sink, cacheDir)
	if err != nil || !result {
		t.Errorf("CheckWASM() failed: %v", err)
	}

	// Verify files were created on real filesystem
	wasmPath := filepath.Join(tmpDir, "static/wasm/search.wasm")
	if _, err := os.Stat(wasmPath); os.IsNotExist(err) {
		t.Errorf("WASM file not created at %s", wasmPath)
	}
}

// TestCheckWASMFs tests the CheckWASMFs function
func TestCheckWASMFs(t *testing.T) {
	fs := afero.NewMemMapFs()
	outputDir := "/output"
	cacheDir := "/cache"
	sink := newFsSink(outputDir, fs)

	result, err := CheckWASMFs(fs, sink, cacheDir)
	if err != nil || !result {
		t.Errorf("CheckWASMFs() failed: %v", err)
	}
}
