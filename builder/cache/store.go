package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/utils"

	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/xxh3"
)

// level3EncoderPool pools level-3 zstd encoders for better performance
var level3EncoderPool = sync.Pool{
	New: func() any {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			// Return a marker that creation failed - callers must check
			return err
		}
		return enc
	},
}

// Store provides content-addressed file storage with two-tier sharding
type Store struct {
	basePath string
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
	dirCache sync.Map
}

var storeTempCounter atomic.Uint64

// NewStore creates a new content-addressed store
func NewStore(basePath string) (*Store, error) {
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

// Close releases resources
func (s *Store) Close() error {
	var errs []error
	if err := s.encoder.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close encoder: %w", err))
	}
	// Decoder.Close() doesn't return an error
	s.decoder.Close()
	if len(errs) > 0 {
		return fmt.Errorf("store close errors: %v", errs)
	}
	return nil
}

// shardPath computes the two-tier shard path: hash[0:2]/hash
func (s *Store) shardPath(category string, hash string) string {
	if len(hash) < 2 {
		return filepath.Join(s.basePath, category, hash)
	}
	return filepath.Join(s.basePath, category, hash[0:2], hash)
}

func extension(ct CompressionType) string {
	if ct == CompressionNone {
		return ".raw"
	}
	return ".zst"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// determineCompression decides compression strategy based on size
func determineCompression(size int) CompressionType {
	if size < utils.RawThreshold {
		return CompressionNone
	}
	if size < utils.FastZstdMax {
		return CompressionZstdFast
	}
	return CompressionZstdLevel3
}

// ensureDir creates a directory and its parents if they haven't been created yet in this session.
// It uses an in-memory cache to avoid expensive redundant syscalls for parent directories.
func (s *Store) ensureDir(dir string) error {
	if _, ok := s.dirCache.Load(dir); ok {
		return nil
	}

	// Collect missing segments by walking upwards
	var missing []string
	curr := filepath.Clean(dir)
	for {
		// Stop if we hit a cached directory
		if _, ok := s.dirCache.Load(curr); ok {
			break
		}

		parent := filepath.Dir(curr)
		// Stop if we reach the root or cannot go further
		if curr == parent || curr == "." || curr == string(filepath.Separator) || curr == filepath.VolumeName(curr) {
			break
		}

		missing = append(missing, curr)
		curr = parent
	}

	// Create missing segments from top to bottom
	for i := len(missing) - 1; i >= 0; i-- {
		p := missing[i]

		// To avoid complex per-path synchronization, we use a small pool of mutexes
		// based on the path hash. This significantly reduces contention compared
		// to a global lock while remaining simple and safe.
		mu := s.getDirMutex(p)
		mu.Lock()
		if _, ok := s.dirCache.Load(p); !ok {
			if err := os.Mkdir(p, 0755); err != nil && !os.IsExist(err) {
				mu.Unlock()
				return err
			}
			s.dirCache.Store(p, struct{}{})
		}
		mu.Unlock()
	}
	return nil
}

var dirMutexes [256]sync.Mutex

func (s *Store) getDirMutex(path string) *sync.Mutex {
	h := xxh3.HashString(path)
	return &dirMutexes[h%256]
}

// Put stores content and returns its hash and compression type
func (s *Store) Put(category string, content []byte) (hash string, ct CompressionType, err error) {
	hash = HashContent(content)
	ct = determineCompression(len(content))

	path := s.shardPath(category, hash) + extension(ct)

	// Ensure directory exists first (combine directory creation with existence check)
	dir := filepath.Dir(path)
	if err := s.ensureDir(dir); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if already exists
	if _, err := os.Stat(path); err == nil {
		return hash, ct, nil
	}

	// Prepare content
	var data []byte
	if ct != CompressionNone {
		// Compress
		if ct == CompressionZstdLevel3 {
			enc := level3EncoderPool.Get()
			var zstdEnc *zstd.Encoder
			if enc == nil {
				// Pool was empty, create new encoder
				var encErr error
				zstdEnc, encErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
				if encErr != nil {
					return "", 0, fmt.Errorf("failed to create zstd encoder: %w", encErr)
				}
			} else if poolErr, ok := enc.(error); ok {
				// Pool creation previously failed, create new encoder
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

	// Atomic write: unique .tmp -> close -> rename
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
	if err := utils.RenameWithRetry(tmpPath, path, 6, 10*time.Millisecond); err != nil {
		if fileExists(path) {
			_ = os.Remove(tmpPath)
			return hash, ct, nil
		}
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to rename file: %w", err)
	}

	return hash, ct, nil
}

// Get retrieves content by hash
func (s *Store) Get(category string, hash string, compressed bool) ([]byte, error) {
	var path string
	if compressed {
		path = s.shardPath(category, hash) + ".zst"
	} else {
		path = s.shardPath(category, hash) + ".raw"
	}

	// Try to find the file
	data, err := os.ReadFile(path)
	if err != nil {
		// Try the other extension
		if compressed {
			path = s.shardPath(category, hash) + ".raw"
		} else {
			path = s.shardPath(category, hash) + ".zst"
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("artifact not found: %s", hash)
		}
		compressed = !compressed
	}

	if compressed {
		return s.decoder.DecodeAll(data, nil)
	}
	return data, nil
}

// Exists checks if a hash exists in the store
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

// Delete removes a hash from the store
func (s *Store) Delete(category string, hash string) error {
	rawPath := s.shardPath(category, hash) + ".raw"
	zstPath := s.shardPath(category, hash) + ".zst"

	_ = os.Remove(rawPath)
	_ = os.Remove(zstPath)
	return nil
}

func (s *Store) ListHashes(category string) ([]string, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return nil, nil
	}

	var hashes []string
	err := filepath.WalkDir(categoryPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Extract hash from filename
		name := d.Name()
		if ext := filepath.Ext(name); ext == ".raw" || ext == ".zst" {
			hash := strings.TrimSuffix(name, ext)
			hashes = append(hashes, hash)
		}
		return nil
	})
	return hashes, err
}

func (s *Store) Size(category string) (int64, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return 0, nil
	}

	var total int64
	err := filepath.WalkDir(categoryPath, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// CleanOrphans deletes artifacts in a category that are older than maxAge and not in liveHashes
func (s *Store) CleanOrphans(category string, liveHashes map[string]bool, maxAge time.Duration) (int, int64, error) {
	deleted, freedBytes := 0, int64(0)
	cutoff := time.Now().Add(-maxAge)
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return 0, 0, nil
	}

	err := filepath.WalkDir(categoryPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		ext := filepath.Ext(d.Name())
		if ext != ".raw" && ext != ".zst" && ext != ".tmp" && ext != ".kosh-backup" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		hash := strings.TrimSuffix(d.Name(), ext)
		if !liveHashes[hash] && info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil {
				deleted++
				freedBytes += info.Size()
			}
		}
		return nil
	})

	return deleted, freedBytes, err
}
