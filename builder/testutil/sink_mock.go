package testutil

import (
	"bytes"
	"io"
	"path/filepath"
	"sync"
	"time"
)

// MemSink is a simple in-memory implementation of ArtifactSink for testing
type MemSink struct {
	Files map[string][]byte
	mu    sync.RWMutex
}

func NewMemSink() *MemSink {
	return &MemSink{Files: make(map[string][]byte)}
}

func (m *MemSink) WriteFile(path string, data []byte) error {
	path = filepath.ToSlash(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Files[path] = data
	return nil
}

func (m *MemSink) WriteStream(path string, fn func(io.Writer) error) error {
	path = filepath.ToSlash(path)
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		return err
	}
	return m.WriteFile(path, buf.Bytes())
}

func (m *MemSink) MkdirAll(path string) error { return nil }
func (m *MemSink) Register(path string)       {}
func (m *MemSink) GetWrittenFiles() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]bool)
	for k := range m.Files {
		res[k] = true
	}
	return res
}
func (m *MemSink) GetOutputDir() string { return "" }
func (m *MemSink) WriteHardlink(src, dst string) (bool, error) {
	return false, nil
}
func (m *MemSink) SetMtime(path string, mtime time.Time) error { return nil }

// MockTransaction is a no-op implementation of BuildTransaction for testing
type MockTransaction struct {
	stagingDir string
	committed  bool
}

func NewMockTransaction(stagingDir string) *MockTransaction {
	return &MockTransaction{stagingDir: stagingDir}
}

func (m *MockTransaction) StagingDir() string { return m.stagingDir }
func (m *MockTransaction) Commit() error {
	m.committed = true
	return nil
}
func (m *MockTransaction) Rollback() error             { return nil }
func (m *MockTransaction) GetLastBuildTime() time.Time { return time.Time{} }

// FailingSink is a sink that always returns errors
type FailingSink struct {
	MemSink
	Err error
}

func (f *FailingSink) WriteFile(path string, data []byte) error {
	return f.Err
}

func (f *FailingSink) WriteStream(path string, fn func(io.Writer) error) error {
	return f.Err
}

func (f *FailingSink) MkdirAll(path string) error {
	return f.Err
}
