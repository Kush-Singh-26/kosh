package testutil

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemSink is a simple in-memory implementation of fspkg.ArtifactSink for testing.
type MemSink struct {
	Files     map[string][]byte
	OutputDir string
	mu        sync.RWMutex
}

// NewMemSink returns a MemSink with an empty file map.
func NewMemSink() *MemSink {
	return &MemSink{Files: make(map[string][]byte)}
}

// NewMemSinkWithDir returns a MemSink with a configured output directory.
func NewMemSinkWithDir(dir string) *MemSink {
	return &MemSink{Files: make(map[string][]byte), OutputDir: dir}
}

// WriteFile stores data at the given path.
func (m *MemSink) WriteFile(path string, data []byte) error {
	path = filepath.ToSlash(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Files[path] = data
	return nil
}

// WriteStream writes data produced by fn into the sink.
func (m *MemSink) WriteStream(path string, fn func(io.Writer) error) error {
	path = filepath.ToSlash(path)
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		return err
	}
	return m.WriteFile(path, buf.Bytes())
}

// MkdirAll is a no-op for MemSink.
func (m *MemSink) MkdirAll(path string) error { return nil }

// Register is a no-op for MemSink.
func (m *MemSink) Register(path string) {}

// GetWrittenFiles returns a set of written file paths.
func (m *MemSink) GetWrittenFiles() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]bool)
	for k := range m.Files {
		res[k] = true
	}
	return res
}

// GetOutputDir returns the configured output directory.
func (m *MemSink) GetOutputDir() string { return m.OutputDir }

// WriteHardlink is a no-op for MemSink.
func (m *MemSink) WriteHardlink(src, dst string) (bool, error) {
	return false, nil
}

// SetMtime is a no-op for MemSink.
func (m *MemSink) SetMtime(path string, mtime time.Time) error { return nil }

// Stat returns a synthetic FileInfo for the stored path.
func (m *MemSink) Stat(path string) (os.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	path = filepath.ToSlash(path)
	data, ok := m.Files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &memFileInfo{name: filepath.Base(path), size: int64(len(data))}, nil
}

// CopyFile copies a stored file to a new destination in the sink.
func (m *MemSink) CopyFile(src, dst string) error {
	dst = filepath.ToSlash(dst)
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.Files[filepath.ToSlash(src)]; ok {
		m.Files[dst] = data
		return nil
	}
	return nil
}

// MockTransaction is a no-op implementation of BuildTransaction for testing.
type MockTransaction struct {
	stagingDir string
	Committed  bool
}

// NewMockTransaction returns a mock transaction with the given staging dir.
func NewMockTransaction(stagingDir string) *MockTransaction {
	return &MockTransaction{stagingDir: stagingDir}
}

// StagingDir returns the staging directory.
func (m *MockTransaction) StagingDir() string { return m.stagingDir }

// Commit marks the transaction as committed.
func (m *MockTransaction) Commit(ctx context.Context) error {
	m.Committed = true
	return nil
}

// Rollback is a no-op for MockTransaction.
func (m *MockTransaction) Rollback() error { return nil }

// GetLastBuildTime returns a zero time for MockTransaction.
func (m *MockTransaction) GetLastBuildTime() time.Time { return time.Time{} }

type memFileInfo struct {
	name string
	size int64
}

// Name returns the base name of the file.
func (f *memFileInfo) Name() string { return f.name }

// Size returns the file size in bytes.
func (f *memFileInfo) Size() int64 { return f.size }

// Mode returns the file mode bits.
func (f *memFileInfo) Mode() os.FileMode { return 0644 }

// ModTime returns a synthetic modification time.
func (f *memFileInfo) ModTime() time.Time { return time.Now() }

// IsDir reports whether the file is a directory.
func (f *memFileInfo) IsDir() bool { return false }

// Sys returns underlying data source (none for MemSink).
func (f *memFileInfo) Sys() any { return nil }

// FailingSink is a sink that always returns errors.
type FailingSink struct {
	MemSink
	Err error
}

// WriteFile returns the configured error.
func (f *FailingSink) WriteFile(path string, data []byte) error {
	return f.Err
}

// WriteStream returns the configured error.
func (f *FailingSink) WriteStream(path string, fn func(io.Writer) error) error {
	return f.Err
}

// MkdirAll returns the configured error.
func (f *FailingSink) MkdirAll(path string) error {
	return f.Err
}

// Stat returns the configured error.
func (f *FailingSink) Stat(path string) (os.FileInfo, error) {
	return nil, f.Err
}
