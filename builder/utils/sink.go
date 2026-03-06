package utils

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ArtifactSink provides an interface for streaming file writes during the build process.
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, fn func(io.Writer) error) error
	MkdirAll(path string) error
	Register(path string)
	GetWrittenFiles() map[string]bool
	GetOutputDir() string
}

type DiskSink struct {
	stagingDir    string
	realOutputDir string
	writtenPaths  sync.Map
	dirCache      sync.Map
	bufPool       sync.Pool
}

func NewDiskSink(stagingDir, realOutputDir string) *DiskSink {
	return &DiskSink{
		stagingDir:    filepath.Clean(stagingDir),
		realOutputDir: filepath.Clean(realOutputDir),
		bufPool: sync.Pool{
			New: func() interface{} {
				// 64KB buffer for streaming
				return bufio.NewWriterSize(nil, 64*1024)
			},
		},
	}
}

func (s *DiskSink) resolvePath(p string) string {
	cleanP := filepath.Clean(p)

	// If the path is absolute and starts with the real output dir, remap it to staging dir
	if filepath.IsAbs(cleanP) {
		if strings.HasPrefix(strings.ToLower(cleanP), strings.ToLower(s.realOutputDir)) {
			rel, err := filepath.Rel(s.realOutputDir, cleanP)
			if err == nil {
				return filepath.Join(s.stagingDir, rel)
			}
		}
		return cleanP
	}

	// If it's a relative path, assume it's relative to stagingDir
	return filepath.Join(s.stagingDir, cleanP)
}

func (s *DiskSink) Register(p string) {
	// Keep track of the final output path (real path) for orphan cleanup and syncing
	cleanP := filepath.Clean(p)
	if !filepath.IsAbs(cleanP) {
		cleanP = filepath.Join(s.realOutputDir, cleanP)
	} else if strings.HasPrefix(strings.ToLower(cleanP), strings.ToLower(s.stagingDir)) {
		rel, err := filepath.Rel(s.stagingDir, cleanP)
		if err == nil {
			cleanP = filepath.Join(s.realOutputDir, rel)
		}
	}
	s.writtenPaths.Store(cleanP, true)
}

func (s *DiskSink) ensureDir(path string) error {
	dir := filepath.Dir(path)
	if _, ok := s.dirCache.Load(dir); ok {
		return nil
	}
	err := os.MkdirAll(dir, 0755)
	if err == nil {
		s.dirCache.Store(dir, true)
	}
	return err
}

func (s *DiskSink) MkdirAll(p string) error {
	target := s.resolvePath(p)
	return os.MkdirAll(target, 0755)
}

func (s *DiskSink) WriteFile(p string, data []byte) error {
	target := s.resolvePath(p)
	if err := s.ensureDir(target); err != nil {
		return err
	}

	err := os.WriteFile(target, data, 0644)
	if err == nil {
		s.Register(p)
	}
	return err
}

func (s *DiskSink) WriteStream(p string, fn func(io.Writer) error) error {
	target := s.resolvePath(p)
	if err := s.ensureDir(target); err != nil {
		return err
	}

	f, err := os.Create(target)
	if err != nil {
		return err
	}

	bw := s.bufPool.Get().(*bufio.Writer)
	bw.Reset(f)

	err = fn(bw)

	if flushErr := bw.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}

	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	// Release the buffer back to the pool, clearing the reference
	bw.Reset(nil)
	s.bufPool.Put(bw)

	if err == nil {
		s.Register(p)
	} else {
		// Try to remove partial file on error
		os.Remove(target)
	}

	return err
}

func (s *DiskSink) GetWrittenFiles() map[string]bool {
	res := make(map[string]bool)
	s.writtenPaths.Range(func(key, value interface{}) bool {
		res[key.(string)] = value.(bool)
		return true
	})
	return res
}

func (s *DiskSink) GetOutputDir() string {
	return s.stagingDir
}
