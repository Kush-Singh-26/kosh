package fs

import (
	"github.com/Kush-Singh-26/kosh/builder/pools"

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
	Stat(path string) (os.FileInfo, error)
}

// DiskSink implements ArtifactSink for disk-based writes with staging directory support
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

// NewDiskSink creates a new DiskSink with staging and real output directories
func NewDiskSink(stagingDir, realOutputDir string) *DiskSink {
	sDir, err := AbsNormalizePath(stagingDir)
	if err != nil {
		sDir = NormalizePath(stagingDir)
	}
	rDir, err := AbsNormalizePath(realOutputDir)
	if err != nil {
		rDir = NormalizePath(realOutputDir)
	}
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
	return s
}

func (s *DiskSink) resolvePathForWrite(p string) (string, error) {
	if cached, ok := s.pathCache.Load(p); ok {
		return cached.(string), nil
	}

	cleanP := NormalizePath(p)
	if strings.Contains(cleanP, "..") {
		return "", fmt.Errorf("refusing to write path with '..': %s", p)
	}

	// Resolve to absolute path for robust comparison
	var absP string
	if filepath.IsAbs(filepath.FromSlash(cleanP)) {
		absP = cleanP
	} else {
		var err error
		absP, err = filepath.Abs(filepath.FromSlash(cleanP))
		if err != nil {
			return "", err
		}
		absP = NormalizePath(absP)
	}

	absPLower := strings.ToLower(absP)
	var resolved string

	if isInsideDir(absPLower, s.stagingDirLower) {
		resolved = absP
	} else if isInsideDir(absPLower, s.realOutputDirLower) {
		rel, err := filepath.Rel(filepath.FromSlash(s.realOutputDir), filepath.FromSlash(absP))
		if err != nil {
			return "", err
		}
		resolved = s.fastJoinStaging(NormalizePath(rel))
	} else if !filepath.IsAbs(filepath.FromSlash(cleanP)) {
		// If it was relative and NOT inside realOutputDir, we assume it's relative to staging root
		resolved = s.fastJoinStaging(cleanP)
	} else {
		return "", fmt.Errorf("refusing to write outside output roots: %s", p)
	}

	s.pathCache.Store(p, resolved)
	return resolved, nil
}

// isInsideDir checks if pathLower is inside dirLower (both must be lowercase and normalized)
func isInsideDir(pathLower, dirLower string) bool {
	if pathLower == dirLower {
		return true
	}
	if !strings.HasPrefix(pathLower, dirLower) {
		return false
	}
	// Ensure it's a sub-path, not just a prefix (e.g. /public and /public-data)
	return len(pathLower) > len(dirLower) && (pathLower[len(dirLower)] == '/' || pathLower[len(dirLower)] == '\\')
}

func (s *DiskSink) fastJoinStaging(rel string) string {
	sb := pools.SharedStringBuilderPool.Get()
	defer pools.SharedStringBuilderPool.Put(sb)
	sb.WriteString(s.stagingDir)
	if len(rel) > 0 && rel[0] != '/' {
		sb.WriteByte('/')
	}
	sb.WriteString(rel)
	return sb.String()
}

func (s *DiskSink) Register(p string) {
	if cached, ok := s.regCache.Load(p); ok {
		s.writtenPaths.Store(cached.(string), true)
		return
	}

	cleanP := NormalizePath(p)
	var finalPath string

	// Resolve to absolute path for consistent tracking
	var absP string
	if filepath.IsAbs(filepath.FromSlash(cleanP)) {
		absP = cleanP
	} else {
		var err error
		absP, err = filepath.Abs(filepath.FromSlash(cleanP))
		if err != nil {
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(s.realOutputDir), filepath.FromSlash(cleanP)))
		} else {
			absP = NormalizePath(absP)
		}
	}

	if absP != "" {
		absPLower := strings.ToLower(absP)
		if isInsideDir(absPLower, s.stagingDirLower) {
			rel, _ := filepath.Rel(filepath.FromSlash(s.stagingDir), filepath.FromSlash(absP))
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(s.realOutputDir), filepath.FromSlash(rel)))
		} else if isInsideDir(absPLower, s.realOutputDirLower) {
			finalPath = absP
		} else if !filepath.IsAbs(filepath.FromSlash(cleanP)) {
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(s.realOutputDir), filepath.FromSlash(cleanP)))
		} else {
			finalPath = absP
		}
	}

	finalPathOS := filepath.FromSlash(finalPath)
	s.regCache.Store(p, finalPathOS)
	s.writtenPaths.Store(finalPathOS, true)
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

	if err := os.MkdirAll(filepath.FromSlash(target), 0755); err != nil {
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

	err = os.WriteFile(filepath.FromSlash(target), data, 0644)
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

	f, err := os.Create(filepath.FromSlash(target))
	if err != nil {
		return err
	}

	bw := s.bufPool.Get().(*bufio.Writer)
	bw.Reset(f)

	var panicked bool
	// Defer cleanup with panic recovery for buffer pool return and partial file removal
	defer func() {
		// Always return buffer to pool, even on panic
		bw.Reset(nil)
		s.bufPool.Put(bw)

		// Close file handle (ignore error, already tracked)
		_ = f.Close()

		// Recover from panic, clean up partial file, and re-panic
		if r := recover(); r != nil {
			panicked = true
			// Try to remove partial file on panic
			_ = os.Remove(filepath.FromSlash(target))
			panic(r) // Re-throw the panic
		}
	}()

	err = fn(bw)

	if flushErr := bw.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}

	// Close file explicitly (defer will also close, but that's safe - os.File.Close is idempotent)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	if err == nil && !panicked {
		s.Register(p)
	} else {
		// Try to remove partial file on error or panic
		_ = os.Remove(filepath.FromSlash(target))
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

// GetRealOutputDir returns the real output directory (non-staging)
func (s *DiskSink) GetRealOutputDir() string {
	return s.realOutputDir
}

func (s *DiskSink) SetMtime(path string, mtime time.Time) error {
	target, err := s.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	return os.Chtimes(filepath.FromSlash(target), mtime, mtime)
}

func (s *DiskSink) Stat(path string) (os.FileInfo, error) {
	target, err := s.resolvePathForWrite(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(filepath.FromSlash(target))
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
	if err := CopyFileInternal(src, filepath.FromSlash(target)); err != nil {
		return err
	}

	s.Register(dst)
	return nil
}
