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

	"github.com/zeebo/xxh3"
)

// ArtifactSink provides an interface for streaming file writes during the build process.
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, fn func(io.Writer) error) error
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
	sDir := filepath.Clean(stagingDir)
	rDir := filepath.Clean(realOutputDir)
	return &DiskSink{
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
}

func (s *DiskSink) resolvePathForWrite(p string) (string, error) {
	if cached, ok := s.pathCache.Load(p); ok {
		return cached.(string), nil
	}

	cleanP := filepath.Clean(p)
	var resolved string

	if filepath.IsAbs(cleanP) {
		// Absolute paths are only allowed when they target the configured output roots.
		if hasPrefixCaseInsensitive(cleanP, s.realOutputDirLower) {
			rel := cleanP[len(s.realOutputDir):]
			if len(rel) > 0 && rel[0] == filepath.Separator {
				rel = rel[1:]
			}
			resolved = s.fastJoinStaging(rel)
		} else if hasPrefixCaseInsensitive(cleanP, s.stagingDirLower) {
			resolved = cleanP
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

// hasPrefixCaseInsensitiveBoth compares two dynamic strings for a prefix match case-insensitively.
func hasPrefixCaseInsensitiveBoth(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c1, c2 := s[i], prefix[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 |= 0x20
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 |= 0x20
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

func isWithinPath(base, target string) bool {
	// Fast path for prefix check without full Clean/Rel overhead
	if hasPrefixCaseInsensitiveBoth(target, base) {
		if len(target) == len(base) {
			return true
		}
		if len(target) > len(base) && (target[len(base)] == filepath.Separator || base[len(base)-1] == filepath.Separator) {
			return true
		}
	}

	// Fallback to more robust check for complex paths
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

	if _, ok := s.dirCache.Load(target); ok {
		return nil
	}

	// Collect missing segments by walking upwards
	var missing []string
	curr := filepath.Clean(target)
	for {
		if _, ok := s.dirCache.Load(curr); ok {
			break
		}

		parent := filepath.Dir(curr)
		if curr == parent || curr == "." || curr == string(filepath.Separator) || curr == filepath.VolumeName(curr) {
			break
		}

		missing = append(missing, curr)
		curr = parent
	}

	// Create missing segments from top to bottom
	for i := len(missing) - 1; i >= 0; i-- {
		p := missing[i]
		mu := s.getDirMutex(p)
		mu.Lock()
		if _, ok := s.dirCache.Load(p); !ok {
			if err := os.Mkdir(p, 0755); err != nil && !os.IsExist(err) {
				mu.Unlock()
				return err
			}
			s.dirCache.Store(p, true)
		}
		mu.Unlock()
	}
	return nil
}

var dirMutexes [256]sync.Mutex

func (s *DiskSink) getDirMutex(path string) *sync.Mutex {
	h := xxh3.HashString(path)
	return &dirMutexes[h%256]
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
