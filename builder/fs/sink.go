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
	writtenPaths       sync.Map // path string -> true
	dirCache           sync.Map // directory path -> struct{}{}
	pathCache          sync.Map // raw input -> resolved staging path
	regCache           sync.Map // raw input -> resolved real path
	bufPool            sync.Pool
}

// NewDiskSink creates a new DiskSink with staging and real output directories
func NewDiskSink(stagingDir, realOutputDir string) *DiskSink {
	stgDir, err := AbsNormalizePath(stagingDir)
	if err != nil {
		stgDir = NormalizePath(stagingDir)
	}
	rlDir, err := AbsNormalizePath(realOutputDir)
	if err != nil {
		rlDir = NormalizePath(realOutputDir)
	}
	sink := &DiskSink{
		stagingDir:         stgDir,
		realOutputDir:      rlDir,
		stagingDirLower:    strings.ToLower(stgDir),
		realOutputDirLower: strings.ToLower(rlDir),
		bufPool: sync.Pool{
			New: func() any {
				// 64KB buffer for streaming writes.
				return bufio.NewWriterSize(nil, copyBufferSize)
			},
		},
	}
	// Seed the directory cache with the staging and real output roots.
	// Use lowercase for case-insensitive matching in the cache.
	return sink
}

func (sink *DiskSink) resolvePathForWrite(path string) (string, error) {
	if cached, ok := sink.pathCache.Load(path); ok {
		return cached.(string), nil
	}

	cleanPath := NormalizePath(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("refusing to write path with '..': %s", path)
	}

	// Resolve to absolute path for robust comparison
	var absPath string
	if filepath.IsAbs(filepath.FromSlash(cleanPath)) {
		absPath = cleanPath
	} else {
		var err error
		absPath, err = filepath.Abs(filepath.FromSlash(cleanPath))
		if err != nil {
			return "", err
		}
		absPath = NormalizePath(absPath)
	}

	absPathLower := strings.ToLower(absPath)
	var resolved string

	switch {
	case isInsideDir(absPathLower, sink.stagingDirLower):
		resolved = absPath
	case isInsideDir(absPathLower, sink.realOutputDirLower):
		rel, err := filepath.Rel(filepath.FromSlash(sink.realOutputDir), filepath.FromSlash(absPath))
		if err != nil {
			return "", err
		}
		resolved = sink.fastJoinStaging(NormalizePath(rel))
	case !filepath.IsAbs(filepath.FromSlash(cleanPath)):
		resolved = sink.fastJoinStaging(cleanPath)
	default:
		return "", fmt.Errorf("refusing to write outside output roots: %s", path)
	}

	sink.pathCache.Store(path, resolved)
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

func (sink *DiskSink) fastJoinStaging(rel string) string {
	pathBuilder := pools.SharedStringBuilderPool.Get()
	defer pools.SharedStringBuilderPool.Put(pathBuilder)
	pathBuilder.WriteString(sink.stagingDir)
	if len(rel) > 0 && rel[0] != '/' {
		pathBuilder.WriteByte('/')
	}
	pathBuilder.WriteString(rel)
	return pathBuilder.String()
}

// Register records a path as written in the real output directory.
func (sink *DiskSink) Register(path string) {
	if cached, ok := sink.regCache.Load(path); ok {
		sink.writtenPaths.Store(cached.(string), true)
		return
	}

	cleanPath := NormalizePath(path)
	var finalPath string

	// Resolve to absolute path for consistent tracking
	var absPath string
	if filepath.IsAbs(filepath.FromSlash(cleanPath)) {
		absPath = cleanPath
	} else {
		var err error
		absPath, err = filepath.Abs(filepath.FromSlash(cleanPath))
		if err != nil {
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(sink.realOutputDir), filepath.FromSlash(cleanPath)))
		} else {
			absPath = NormalizePath(absPath)
		}
	}

	if absPath != "" {
		absPathLower := strings.ToLower(absPath)
		switch {
		case isInsideDir(absPathLower, sink.stagingDirLower):
			rel, _ := filepath.Rel(filepath.FromSlash(sink.stagingDir), filepath.FromSlash(absPath))
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(sink.realOutputDir), filepath.FromSlash(rel)))
		case isInsideDir(absPathLower, sink.realOutputDirLower):
			finalPath = absPath
		case !filepath.IsAbs(filepath.FromSlash(cleanPath)):
			finalPath = NormalizePath(filepath.Join(filepath.FromSlash(sink.realOutputDir), filepath.FromSlash(cleanPath)))
		default:
			finalPath = absPath
		}
	}

	finalPathOS := filepath.FromSlash(finalPath)
	sink.regCache.Store(path, finalPathOS)
	sink.writtenPaths.Store(finalPathOS, true)
}

func (sink *DiskSink) ensureDir(path string) error {
	dir := filepath.Dir(path)
	return sink.MkdirAll(dir)
}

// MkdirAll ensures the directory exists inside the sink output roots.
func (sink *DiskSink) MkdirAll(p string) error {
	target, err := sink.resolvePathForWrite(p)
	if err != nil {
		return err
	}

	targetLower := strings.ToLower(target)
	if _, ok := sink.dirCache.Load(targetLower); ok {
		return nil
	}

	if err := os.MkdirAll(filepath.FromSlash(target), defaultDirMode); err != nil {
		return err
	}

	sink.dirCache.Store(targetLower, true)
	return nil
}

// WriteFile writes a full file into the sink and registers it.
func (sink *DiskSink) WriteFile(path string, data []byte) error {
	target, err := sink.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	if err := sink.ensureDir(target); err != nil {
		return err
	}

	err = os.WriteFile(filepath.FromSlash(target), data, defaultFileMode)
	if err == nil {
		sink.Register(path)
	}
	return err
}

// WriteStream streams content into a file inside the sink and registers it.
func (sink *DiskSink) WriteStream(path string, fn func(io.Writer) error) error {
	target, err := sink.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	if err := sink.ensureDir(target); err != nil {
		return err
	}

	file, err := os.Create(filepath.FromSlash(target))
	if err != nil {
		return err
	}

	bufWriter := sink.bufPool.Get().(*bufio.Writer)
	bufWriter.Reset(file)

	var panicked bool
	// Defer cleanup with panic recovery for buffer pool return and partial file removal
	defer func() {
		// Always return buffer to pool, even on panic
		bufWriter.Reset(nil)
		sink.bufPool.Put(bufWriter)

		// Close file handle (ignore error, already tracked)
		_ = file.Close()

		// Recover from panic, clean up partial file, and re-panic
		if rec := recover(); rec != nil {
			panicked = true
			// Try to remove partial file on panic
			_ = os.Remove(filepath.FromSlash(target))
			panic(rec) // Re-throw the panic
		}
	}()

	err = fn(bufWriter)

	if flushErr := bufWriter.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}

	// Close file explicitly (defer will also close, but that's safe - os.File.Close is idempotent)
	if closeErr := file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	if err == nil && !panicked {
		sink.Register(path)
	} else {
		// Try to remove partial file on error or panic
		_ = os.Remove(filepath.FromSlash(target))
	}

	return err
}

// GetWrittenFiles returns a snapshot of files registered during the build.
func (sink *DiskSink) GetWrittenFiles() map[string]bool {
	res := make(map[string]bool)
	sink.writtenPaths.Range(func(key, value any) bool {
		res[key.(string)] = value.(bool)
		return true
	})
	return res
}

// GetOutputDir returns the staging output directory.
func (sink *DiskSink) GetOutputDir() string {
	return sink.stagingDir
}

// GetRealOutputDir returns the real output directory (non-staging)
func (sink *DiskSink) GetRealOutputDir() string {
	return sink.realOutputDir
}

// SetMtime updates the modification time for a file in the sink.
func (sink *DiskSink) SetMtime(path string, mtime time.Time) error {
	target, err := sink.resolvePathForWrite(path)
	if err != nil {
		return err
	}
	return os.Chtimes(filepath.FromSlash(target), mtime, mtime)
}

// Stat returns os.FileInfo for a path within the sink.
func (sink *DiskSink) Stat(path string) (os.FileInfo, error) {
	target, err := sink.resolvePathForWrite(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(filepath.FromSlash(target))
}

// CopyFile copies a file into the sink and registers it.
func (sink *DiskSink) CopyFile(src, dst string) error {
	target, err := sink.resolvePathForWrite(dst)
	if err != nil {
		return err
	}
	if err := sink.ensureDir(target); err != nil {
		return err
	}

	// Call platform-specific optimized copy
	if err := CopyFileInternal(src, filepath.FromSlash(target)); err != nil {
		return err
	}

	sink.Register(dst)
	return nil
}
