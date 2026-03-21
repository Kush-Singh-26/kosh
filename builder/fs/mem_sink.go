package fs

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemSink implements ArtifactSink for in-memory testing
type MemSink struct {
	Files sync.Map
}

// NewMemSink creates a new in-memory sink for testing
func NewMemSink() *MemSink {
	return &MemSink{}
}

func (s *MemSink) WriteFile(path string, data []byte) error {
	s.Files.Store(path, data)
	return nil
}

func (s *MemSink) WriteStream(path string, fn func(io.Writer) error) error {
	// Simple memory buffer implementation
	var buf byteBuffer
	if err := fn(&buf); err != nil {
		return err
	}
	s.Files.Store(path, buf.Bytes())
	return nil
}

func (s *MemSink) MkdirAll(path string) error { return nil }
func (s *MemSink) Register(path string)       {}
func (s *MemSink) GetWrittenFiles() map[string]bool {
	res := make(map[string]bool)
	s.Files.Range(func(k, v any) bool {
		res[k.(string)] = true
		return true
	})
	return res
}
func (s *MemSink) GetOutputDir() string                        { return "/mem" }
func (s *MemSink) WriteHardlink(src, dst string) (bool, error) { return false, nil }
func (s *MemSink) CopyFile(src, dst string) error {
	// MemSink doesn't have a real filesystem, so we can't "copy" from a path
	// unless we assume src is a real OS path.
	// For tests, we'll just ignore or implementation a mock read if needed.
	return nil
}

func (s *MemSink) SetMtime(path string, mtime time.Time) error { return nil }

func (s *MemSink) Stat(path string) (os.FileInfo, error) {
	data, ok := s.Files.Load(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	b := data.([]byte)
	return &memFileInfo{name: filepath.Base(path), size: int64(len(b))}, nil
}

type memFileInfo struct {
	name string
	size int64
}

func (f *memFileInfo) Name() string       { return f.name }
func (f *memFileInfo) Size() int64        { return f.size }
func (f *memFileInfo) Mode() os.FileMode  { return 0644 }
func (f *memFileInfo) ModTime() time.Time { return time.Now() }
func (f *memFileInfo) IsDir() bool        { return false }
func (f *memFileInfo) Sys() any           { return nil }

type byteBuffer struct {
	data []byte
}

func (b *byteBuffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *byteBuffer) Bytes() []byte { return b.data }
