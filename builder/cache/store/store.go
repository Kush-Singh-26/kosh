package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/retry"

	"github.com/klauspost/compress/zstd"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

// level3EncoderPool stores *zstd.Encoder instances (or errors on init failure).
var level3EncoderPool = sync.Pool{
	New: func() any {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return err
		}
		return enc
	},
}

// Store manages compressed content-addressed blobs on disk.
type Store struct {
	basePath string
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
	dirCache sync.Map // directory path -> struct{}{} (exists)
}

var storeTempCounter atomic.Uint64

// New creates a Store rooted at the provided base path.
func New(basePath string) (*Store, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	return &Store{
		basePath: basePath,
		encoder:  encoder,
		decoder:  decoder,
	}, nil
}

// Close releases store resources.
func (s *Store) Close() error {
	var errs []error
	if err := s.encoder.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close encoder: %w", err))
	}
	s.decoder.Close()
	if len(errs) > 0 {
		return fmt.Errorf("store close errors: %w", errors.Join(errs...))
	}
	return nil
}

func (s *Store) shardPath(category string, hash string) string {
	if len(hash) < 2 {
		return filepath.Join(s.basePath, category, hash)
	}
	return filepath.Join(s.basePath, category, hash[0:2], hash)
}

func extension(ct core.CompressionType) string {
	if ct == core.CompressionNone {
		return ".raw"
	}
	return ".zst"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func determineCompression(size int) core.CompressionType {
	if size < models.RawThreshold {
		return core.CompressionNone
	}
	if size < models.FastZstdMax {
		return core.CompressionZstdFast
	}
	return core.CompressionZstdLevel3
}

func (s *Store) ensureDir(dir string) error {
	// Fast path: check if directory already exists in cache
	if _, ok := s.dirCache.Load(dir); ok {
		return nil
	}

	// Slow path: create directory with proper locking
	mu := s.getDirMutex(dir)
	mu.Lock()
	defer mu.Unlock()

	// Check again after acquiring lock
	if _, ok := s.dirCache.Load(dir); ok {
		return nil
	}

	// Walk up to find missing parents
	var missing []string
	curr := filepath.Clean(dir)
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

	// Create from top to bottom
	for i := len(missing) - 1; i >= 0; i-- {
		p := missing[i]
		if err := os.Mkdir(p, 0755); err != nil && !os.IsExist(err) {
			mu.Unlock()
			return err
		}
		s.dirCache.Store(p, struct{}{})
	}

	return nil
}

var dirMutexes [256]sync.Mutex

func (s *Store) getDirMutex(path string) *sync.Mutex {
	h := xxh3.HashString(path)
	return &dirMutexes[h%256]
}

// Put stores content and returns its hash and compression type.
func (s *Store) Put(category string, content []byte) (hash string, ct core.CompressionType, err error) {
	hash = core.HashContent(content)
	ct = determineCompression(len(content))

	path := s.shardPath(category, hash) + extension(ct)

	dir := filepath.Dir(path)
	if err := s.ensureDir(dir); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		return hash, ct, nil
	}

	var data []byte
	if ct != core.CompressionNone {
		if ct == core.CompressionZstdLevel3 {
			enc := level3EncoderPool.Get()
			var zstdEnc *zstd.Encoder
			if enc == nil {
				var encErr error
				zstdEnc, encErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
				if encErr != nil {
					return "", 0, fmt.Errorf("failed to create zstd encoder: %w", encErr)
				}
			} else if poolErr, ok := enc.(error); ok {
				if poolErr != nil {
					var encErr error
					zstdEnc, encErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
					if encErr != nil {
						return "", 0, fmt.Errorf("failed to create zstd encoder: %w", encErr)
					}
				}
			} else {
				zstdEnc = enc.(*zstd.Encoder)
			}
			data = zstdEnc.EncodeAll(content, nil)
			zstdEnc.Reset(nil)
			level3EncoderPool.Put(zstdEnc)
		} else {
			data = s.encoder.EncodeAll(content, nil)
		}
	} else {
		data = content
	}

	tmpPath := fmt.Sprintf("%s.%s.%d.%d.tmp", path, hash[:8], os.Getpid(), storeTempCounter.Add(1))
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to write content: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to close file: %w", err)
	}

	if fileExists(path) {
		_ = os.Remove(tmpPath)
		return hash, ct, nil
	}

	if _, err := os.Stat(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("temp file missing before rename: %w", err)
	}
	if err := retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        context.Background(),
		OldPath:    tmpPath,
		NewPath:    path,
		MaxRetries: 6,
		BaseDelay:  10 * time.Millisecond,
	}); err != nil {
		if fileExists(path) {
			_ = os.Remove(tmpPath)
			return hash, ct, nil
		}
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to rename file: %w", err)
	}

	return hash, ct, nil
}

// Get retrieves content by hash, decompressing when needed.
func (s *Store) Get(category string, hash string, compressed bool) ([]byte, error) {
	var path string
	if compressed {
		path = s.shardPath(category, hash) + ".zst"
	} else {
		path = s.shardPath(category, hash) + ".raw"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if compressed {
			path = s.shardPath(category, hash) + ".raw"
		} else {
			path = s.shardPath(category, hash) + ".zst"
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, core.ErrNoContent
		}
		compressed = !compressed
	}

	if compressed {
		return s.decoder.DecodeAll(data, nil)
	}
	return data, nil
}

// Exists reports whether the blob exists in the store.
func (s *Store) Exists(category string, hash string) bool {
	rawPath := s.shardPath(category, hash) + ".raw"
	zstPath := s.shardPath(category, hash) + ".zst"

	if _, err := os.Stat(rawPath); err == nil {
		return true
	}
	if _, err := os.Stat(zstPath); err == nil {
		return true
	}
	return false
}

// Delete removes a stored blob by hash.
func (s *Store) Delete(category string, hash string) error {
	rawPath := s.shardPath(category, hash) + ".raw"
	zstPath := s.shardPath(category, hash) + ".zst"

	_ = os.Remove(rawPath)
	_ = os.Remove(zstPath)
	return nil
}

// ListHashes lists all hashes stored under a category.
func (s *Store) ListHashes(category string) ([]string, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return []string{}, nil
	}

	var hashes []string
	var mu sync.Mutex
	err := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         context.Background(),
		SourceFs:    afero.NewOsFs(),
		Root:        categoryPath,
		Concurrency: 0,
		WalkFn: func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			name := info.Name()
			if ext := filepath.Ext(name); ext == ".raw" || ext == ".zst" {
				hash := strings.TrimSuffix(name, ext)
				mu.Lock()
				hashes = append(hashes, hash)
				mu.Unlock()
			}
			return nil
		},
	})
	return hashes, err
}

// Size returns the total byte size for a category.
func (s *Store) Size(category string) (int64, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return 0, nil
	}

	var total int64
	err := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         context.Background(),
		SourceFs:    afero.NewOsFs(),
		Root:        categoryPath,
		Concurrency: 0,
		WalkFn: func(_ string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				atomic.AddInt64(&total, info.Size())
			}
			return nil
		},
	})
	return total, err
}

// CleanOrphans removes stale blobs not referenced by live hashes.
func (s *Store) CleanOrphans(category string, liveHashes map[string]bool, maxAge time.Duration) (int, int64, error) {
	var deleted int64
	var freedBytes int64
	cutoff := time.Now().Add(-maxAge)
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return 0, 0, nil
	}

	err := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         context.Background(),
		SourceFs:    afero.NewOsFs(),
		Root:        categoryPath,
		Concurrency: 0,
		WalkFn: func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			ext := filepath.Ext(info.Name())
			if ext != ".raw" && ext != ".zst" && ext != ".tmp" && ext != ".kosh-backup" {
				return nil
			}

			hash := strings.TrimSuffix(info.Name(), ext)
			if !liveHashes[hash] && info.ModTime().Before(cutoff) {
				if err := os.Remove(path); err == nil {
					atomic.AddInt64(&deleted, 1)
					atomic.AddInt64(&freedBytes, info.Size())
				}
			}
			return nil
		},
	})

	return int(deleted), freedBytes, err
}
