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

const (
	storeDirMode       = 0755
	hashShardPrefixLen = 2
	hashPrefixLen      = 8
	dirMutexBuckets    = 256
	renameMaxRetries   = 6
	renameBaseDelay    = 10 * time.Millisecond
)

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
func (storeInstance *Store) Close() error {
	var errs []error
	if err := storeInstance.encoder.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close encoder: %w", err))
	}
	storeInstance.decoder.Close()
	if len(errs) > 0 {
		return fmt.Errorf("store close errors: %w", errors.Join(errs...))
	}
	return nil
}

func (storeInstance *Store) shardPath(category string, hash string) string {
	if len(hash) < hashShardPrefixLen {
		return filepath.Join(storeInstance.basePath, category, hash)
	}
	return filepath.Join(storeInstance.basePath, category, hash[0:hashShardPrefixLen], hash)
}

func extension(compressionType core.CompressionType) string {
	if compressionType == core.CompressionNone {
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

func (storeInstance *Store) ensureDir(dir string) error {
	// Fast path: check if directory already exists in cache
	if _, ok := storeInstance.dirCache.Load(dir); ok {
		return nil
	}

	// Slow path: create directory with proper locking
	mutex := storeInstance.getDirMutex(dir)
	mutex.Lock()
	defer mutex.Unlock()

	// Check again after acquiring lock
	if _, ok := storeInstance.dirCache.Load(dir); ok {
		return nil
	}

	// Walk up to find missing parents
	var missing []string
	currentDir := filepath.Clean(dir)
	for {
		if _, ok := storeInstance.dirCache.Load(currentDir); ok {
			break
		}
		parent := filepath.Dir(currentDir)
		if currentDir == parent || currentDir == "." || currentDir == string(filepath.Separator) || currentDir == filepath.VolumeName(currentDir) {
			break
		}
		missing = append(missing, currentDir)
		currentDir = parent
	}

	// Create from top to bottom
	for i := len(missing) - 1; i >= 0; i-- {
		path := missing[i]
		if err := os.Mkdir(path, storeDirMode); err != nil && !os.IsExist(err) {
			return err
		}
		storeInstance.dirCache.Store(path, struct{}{})
	}

	return nil
}

var dirMutexes [dirMutexBuckets]sync.Mutex

func (storeInstance *Store) getDirMutex(path string) *sync.Mutex {
	hash := xxh3.HashString(path)
	return &dirMutexes[hash%dirMutexBuckets]
}

// Put stores content and returns its hash and compression type.
func (storeInstance *Store) Put(category string, content []byte) (string, core.CompressionType, error) {
	hash := core.HashContent(content)
	compressionType := determineCompression(len(content))

	path := storeInstance.shardPath(category, hash) + extension(compressionType)

	dir := filepath.Dir(path)
	if err := storeInstance.ensureDir(dir); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	if _, err := os.Stat(path); err == nil {
		return hash, compressionType, nil
	}

	data, err := storeInstance.compressContent(content, compressionType)
	if err != nil {
		return "", 0, err
	}

	if err := storeInstance.writeBlob(path, hash, data); err != nil {
		return "", 0, err
	}

	return hash, compressionType, nil
}

// Get retrieves content by hash, decompressing when needed.
func (storeInstance *Store) Get(category string, hash string, compressed bool) ([]byte, error) {
	var path string
	if compressed {
		path = storeInstance.shardPath(category, hash) + ".zst"
	} else {
		path = storeInstance.shardPath(category, hash) + ".raw"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if compressed {
			path = storeInstance.shardPath(category, hash) + ".raw"
		} else {
			path = storeInstance.shardPath(category, hash) + ".zst"
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, core.ErrNoContent
		}
		compressed = !compressed
	}

	if compressed {
		return storeInstance.decoder.DecodeAll(data, nil)
	}
	return data, nil
}

// Exists reports whether the blob exists in the store.
func (storeInstance *Store) Exists(category string, hash string) bool {
	rawPath := storeInstance.shardPath(category, hash) + ".raw"
	zstPath := storeInstance.shardPath(category, hash) + ".zst"

	if _, err := os.Stat(rawPath); err == nil {
		return true
	}
	if _, err := os.Stat(zstPath); err == nil {
		return true
	}
	return false
}

// Delete removes a stored blob by hash.
func (storeInstance *Store) Delete(category string, hash string) error {
	rawPath := storeInstance.shardPath(category, hash) + ".raw"
	zstPath := storeInstance.shardPath(category, hash) + ".zst"

	_ = os.Remove(rawPath)
	_ = os.Remove(zstPath)
	return nil
}

// ListHashes lists all hashes stored under a category.
func (storeInstance *Store) ListHashes(category string) ([]string, error) {
	categoryPath := filepath.Join(storeInstance.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return []string{}, nil
	}

	var hashes []string
	var mutex sync.Mutex
	err := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         context.Background(),
		SourceFs:    afero.NewOsFs(),
		Root:        categoryPath,
		Concurrency: 0,
		WalkFn: func(_ string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			name := info.Name()
			if ext := filepath.Ext(name); ext == ".raw" || ext == ".zst" {
				hash := strings.TrimSuffix(name, ext)
				mutex.Lock()
				hashes = append(hashes, hash)
				mutex.Unlock()
			}
			return nil
		},
	})
	return hashes, err
}

// Size returns the total byte size for a category.
func (storeInstance *Store) Size(category string) (int64, error) {
	categoryPath := filepath.Join(storeInstance.basePath, category)
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
func (storeInstance *Store) CleanOrphans(category string, liveHashes map[string]bool, maxAge time.Duration) (int, int64, error) {
	var deleted int64
	var freedBytes int64
	cutoff := time.Now().Add(-maxAge)
	categoryPath := filepath.Join(storeInstance.basePath, category)
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

func (storeInstance *Store) compressContent(content []byte, compressionType core.CompressionType) ([]byte, error) {
	if compressionType == core.CompressionNone {
		return content, nil
	}

	if compressionType == core.CompressionZstdLevel3 {
		encoder := level3EncoderPool.Get()
		var zstdEncoderInstance *zstd.Encoder
		if encoder == nil {
			var err error
			zstdEncoderInstance, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
			if err != nil {
				return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
			}
		} else if _, ok := encoder.(error); ok {
			var err error
			zstdEncoderInstance, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
			if err != nil {
				return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
			}
		} else {
			zstdEncoderInstance = encoder.(*zstd.Encoder)
		}
		defer func() {
			zstdEncoderInstance.Reset(nil)
			level3EncoderPool.Put(zstdEncoderInstance)
		}()
		return zstdEncoderInstance.EncodeAll(content, nil), nil
	}

	return storeInstance.encoder.EncodeAll(content, nil), nil
}

func (storeInstance *Store) writeBlob(path, hash string, data []byte) error {
	tmpPath := fmt.Sprintf("%s.%s.%d.%d.tmp", path, hash[:hashPrefixLen], os.Getpid(), storeTempCounter.Add(1))
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write content: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close file: %w", err)
	}

	if fileExists(path) {
		_ = os.Remove(tmpPath)
		return nil
	}

	if err := retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        context.Background(),
		OldPath:    tmpPath,
		NewPath:    path,
		MaxRetries: renameMaxRetries,
		BaseDelay:  renameBaseDelay,
	}); err != nil {
		_ = os.Remove(tmpPath)
		if fileExists(path) {
			return nil
		}
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}
