package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Store provides content-addressed file storage with two-tier sharding
type Store struct {
	basePath string
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
}

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
	_ = s.encoder.Close()
	s.decoder.Close()
	return nil
}

// shardPath computes the two-tier shard path: hash[0:2]/hash[2:4]/hash
func (s *Store) shardPath(category string, hash string) string {
	if len(hash) < 4 {
		return filepath.Join(s.basePath, category, hash)
	}
	return filepath.Join(s.basePath, category, hash[0:2], hash[2:4], hash)
}

// extension returns the file extension based on compression type
func extension(ct CompressionType) string {
	if ct == CompressionNone {
		return ".raw"
	}
	return ".zst"
}

// determineCompression decides compression strategy based on size
func determineCompression(size int) CompressionType {
	if size < RawThreshold {
		return CompressionNone
	}
	if size < FastZstdMax {
		return CompressionZstdFast
	}
	return CompressionZstdLevel3
}

// Put stores content and returns its hash and compression type
func (s *Store) Put(category string, content []byte) (hash string, ct CompressionType, err error) {
	hash = HashContent(content)
	ct = determineCompression(len(content))

	path := s.shardPath(category, hash) + extension(ct)

	// Check if already exists
	if _, err := os.Stat(path); err == nil {
		return hash, ct, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Prepare content
	var data []byte
	if ct != CompressionNone {
		// Compress
		if ct == CompressionZstdLevel3 {
			enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
			if err != nil {
				return "", 0, err
			}
			data = enc.EncodeAll(content, nil)
			_ = enc.Close()
		} else {
			data = s.encoder.EncodeAll(content, nil)
		}
	} else {
		data = content
	}

	// Atomic write: .tmp -> fsync -> rename
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to write content: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to sync file: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
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

// ListHashes returns all hashes in a category
func (s *Store) ListHashes(category string) ([]string, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return nil, nil
	}

	var hashes []string
	err := filepath.Walk(categoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Extract hash from filename
		name := info.Name()
		if ext := filepath.Ext(name); ext == ".raw" || ext == ".zst" {
			hash := strings.TrimSuffix(name, ext)
			hashes = append(hashes, hash)
		}
		return nil
	})
	return hashes, err
}

// Size returns total bytes used by a category
func (s *Store) Size(category string) (int64, error) {
	categoryPath := filepath.Join(s.basePath, category)
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		return 0, nil
	}

	var total int64
	err := filepath.Walk(categoryPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// CopyTo copies a hash to a writer
func (s *Store) CopyTo(category string, hash string, compressed bool, w io.Writer) error {
	data, err := s.Get(category, hash, compressed)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
