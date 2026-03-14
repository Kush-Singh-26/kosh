package utils

import (
	"bufio"
	"fmt"
	"io"
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
	CopyFile(srcPath, destPath string) error
	MkdirAll(path string) error
	Register(path string)
	GetWrittenFiles() map[string]bool
	GetOutputDir() string
	SetMtime(path string, mtime time.Time) error
}

type DiskSink struct {
	stagingDir         string
	realOutputDir      string
	stagingDirLower    string
	realOutputDirLower string
	writtenPaths       sync.Map
	dirCache           sync.Map
	pathCache          sync.Map // raw input -> resolved staging path
	regCache           sync.Map // raw input -> resolved real path
	bufPool            sync.Pool
}

func NewDiskSink(stagingDir, realOutputDir string) *DiskSink {
	sDir, _ := filepath.Abs(stagingDir)
	rDir, _ := filepath.Abs(realOutputDir)
	sDir = filepath.Clean(sDir)
	rDir = filepath.Clean(rDir)
	s := &DiskSink{
		stagingDir:         sDir,
		realOutputDir:      rDir,
		stagingDirLower:    strings.ToLower(sDir),
		realOutputDirLower: strings.ToLower(rDir),
		bufPool: sync.Pool{
			New: func() any {
				// 64KB buffer for streaming
				return bufio.NewWriterSize(nil, 64*1024)
			},
		},
	}
	// Seed the directory cache with the staging and real output roots.
	// Use lowercase for case-insensitive matching in the cache.
	s.dirCache.Store(strings.ToLower(sDir), true)
	s.dirCache.Store(strings.ToLower(rDir), true)
	return s
}

func (s *DiskSink) resolvePathForWrite(p string) (string, error) {
	if cached, ok := s.pathCache.Load(p); ok {
		return cached.(string), nil
	}

	cleanP := filepath.Clean(p)
	var resolved string

	if filepath.IsAbs(cleanP) {
		// Resolve and validate absolute paths before checking prefixes
		resolvedAbs, err := filepath.Abs(cleanP)
		if err != nil {
			return "", err
		}
		resolvedAbs = filepath.Clean(resolvedAbs)

		// Check if the resolved path is within allowed roots
		if hasPrefixCaseInsensitive(resolvedAbs, s.realOutputDirLower) {
			rel, err := filepath.Rel(s.realOutputDir, resolvedAbs)
			if err != nil {
				return "", err
			}
			resolved = s.fastJoinStaging(rel)
		} else if hasPrefixCaseInsensitive(resolvedAbs, s.stagingDirLower) {
			resolved = resolvedAbs
		} else {
			return "", fmt.Errorf("refusing to write outside output roots: %s", p)
		}
	} else {
		resolved = s.fastJoinStaging(cleanP)
	}

	s.pathCache.Store(p, resolved)
	return resolved, nil
}

func (s *DiskSink) fastJoinStaging(rel string) string {
	sb := SharedStringBuilderPool.Get()
	defer SharedStringBuilderPool.Put(sb)
	sb.WriteString(s.stagingDir)
	if len(rel) > 0 && rel[0] != filepath.Separator {
		sb.WriteByte(filepath.Separator)
	}
	sb.WriteString(rel)
	return sb.String()
}

// hasPrefixCaseInsensitive checks if s starts with prefixLower (which must be lowercase).
func hasPrefixCaseInsensitive(s, prefixLower string) bool {
	if len(s) < len(prefixLower) {
		return false
	}
	for i := 0; i < len(prefixLower); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c |= 0x20 // Fast ASCII lowercase
		}
		if c != prefixLower[i] {
			return false
		}
	}
	return true
}

func (s *DiskSink) Register(p string) {
	if cached, ok := s.regCache.Load(p); ok {
		s.writtenPaths.Store(cached.(string), true)
		return
	}

	// Keep track of the final output path (real path) for orphan cleanup and syncing
	cleanP := filepath.Clean(p)
	var finalPath string
	if !filepath.IsAbs(cleanP) {
		finalPath = filepath.Join(s.realOutputDir, cleanP)
	} else {
		if hasPrefixCaseInsensitive(cleanP, s.stagingDirLower) {
			rel := cleanP[len(s.stagingDir):]
			if len(rel) > 0 && rel[0] == filepath.Separator {
				rel = rel[1:]
			}
			finalPath = filepath.Join(s.realOutputDir, rel)
		} else {
			finalPath = cleanP
		}
	}
	s.regCache.Store(p, finalPath)
	s.writtenPaths.Store(finalPath, true)
}

func (s *DiskSink) ensureDir(path string) error {
	dir := filepath.Dir(path)
	return s.MkdirAll(dir)
}

func (s *DiskSink) MkdirAll(p string) error {
	target, err := s.resolvePathForWrite(p)
	if err != nil {
		return err
	}

	targetLower := strings.ToLower(target)
	if _, ok := s.dirCache.Load(targetLower); ok {
		return nil
	}

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	s.dirCache.Store(targetLower, true)
	return nil
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
		_ = os.Remove(target)
	}

	return err
}

func (s *DiskSink) GetWrittenFiles() map[string]bool {
	res := make(map[string]bool)
	s.writtenPaths.Range(func(key, value any) bool {
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

func (s *DiskSink) SetMtime(path string, mtime time.Time) error {
	target, err := s.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	return os.Chtimes(target, mtime, mtime)
}

func (s *DiskSink) CopyFile(src, dst string) error {
	target, err := s.resolvePathForWrite(dst)
	if err != nil {
		return err
	}
	if err := s.ensureDir(target); err != nil {
		return err
	}

	// Call platform-specific optimized copy
	if err := CopyFileInternal(src, target); err != nil {
		return err
	}

	s.Register(dst)
	return nil
}
