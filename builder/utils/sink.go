package utils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ArtifactSink provides an interface for streaming file writes during the build process.
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, fn func(io.Writer) error) error
	MkdirAll(path string) error
	Register(path string)
	GetWrittenFiles() map[string]bool
	GetOutputDir() string
	WriteHardlink(src, dst string) (bool, error)
	SetMtime(path string, mtime time.Time) error
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

func isWithinPath(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)

	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (s *DiskSink) resolvePathForWrite(p string) (string, error) {
	cleanP := filepath.Clean(p)

	if filepath.IsAbs(cleanP) {
		// Absolute paths are only allowed when they target the configured output roots.
		if isWithinPath(s.realOutputDir, cleanP) {
			rel, err := filepath.Rel(s.realOutputDir, cleanP)
			if err != nil {
				return "", fmt.Errorf("failed to resolve output-relative path %s: %w", p, err)
			}
			return filepath.Join(s.stagingDir, rel), nil
		}
		if isWithinPath(s.stagingDir, cleanP) {
			return cleanP, nil
		}
		return "", fmt.Errorf("refusing to write outside output roots: %s", p)
	}

	return filepath.Join(s.stagingDir, cleanP), nil
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
	target, err := s.resolvePathForWrite(p)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0755)
}

func (s *DiskSink) WriteFile(p string, data []byte) error {
	target, err := s.resolvePathForWrite(p)
	if err != nil {
		return err
	}
	if err := s.ensureDir(target); err != nil {
		return err
	}

	err = os.WriteFile(target, data, 0644)
	if err == nil {
		s.Register(p)
	}
	return err
}

func (s *DiskSink) WriteStream(p string, fn func(io.Writer) error) error {
	target, err := s.resolvePathForWrite(p)
	if err != nil {
		return err
	}
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

func (s *DiskSink) GetRealOutputDir() string {
	return s.realOutputDir
}

func (s *DiskSink) WriteHardlink(src, dst string) (bool, error) {
	target, err := s.resolvePathForWrite(dst)
	if err != nil {
		return false, err
	}

	// Construct real output path for comparison
	realOutputPath := dst
	if !filepath.IsAbs(realOutputPath) {
		realOutputPath = filepath.Join(s.realOutputDir, realOutputPath)
	} else if isWithinPath(s.stagingDir, realOutputPath) {
		rel, err := filepath.Rel(s.stagingDir, realOutputPath)
		if err != nil {
			return false, err
		}
		realOutputPath = filepath.Join(s.realOutputDir, rel)
	} else if !isWithinPath(s.realOutputDir, realOutputPath) {
		return false, fmt.Errorf("refusing to hardlink outside output roots: %s", dst)
	}

	// Compare source against previous build's output
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false, nil
	}

	realInfo, err := os.Stat(realOutputPath)
	if err == nil {
		if srcInfo.Size() == realInfo.Size() {
			timeDiff := srcInfo.ModTime().Unix() - realInfo.ModTime().Unix()
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			if timeDiff <= 1 {
				if err := s.ensureDir(target); err != nil {
					return false, err
				}
				if err := os.Link(src, target); err == nil {
					s.Register(dst)
					return true, nil
				} else {
					slog.Debug("Hardlink failed", "src", src, "target", target, "error", err)
				}
			} else {
				slog.Debug("Mtime mismatch", "src", src, "srcMtime", srcInfo.ModTime(), "realMtime", realInfo.ModTime(), "diff", timeDiff)
			}
		} else {
			slog.Debug("Size mismatch", "src", src, "srcSize", srcInfo.Size(), "realSize", realInfo.Size())
		}
	} else {
		slog.Debug("Real output not found", "path", realOutputPath)
	}

	return false, nil
}

func (s *DiskSink) SetMtime(path string, mtime time.Time) error {
	target, err := s.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	return os.Chtimes(target, mtime, mtime)
}
