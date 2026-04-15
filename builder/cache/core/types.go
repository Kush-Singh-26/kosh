// Package cache provides a BoltDB + content-addressed filesystem cache for Kosh SSG.
package core

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tinylib/msgp/msgp"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// ErrNoContent indicates a cache miss when reading content.
var ErrNoContent = errors.New("no content found in cache")

// PostMeta stores metadata about a cached post
type PostMeta = models.PostMeta

// SSRArtifact stores server-side rendered content (D2 diagrams, KaTeX math)
type SSRArtifact = models.SSRArtifact

// SearchRecord stores pre-computed search data for BM25
type SearchRecord = models.SearchRecord

// Dependencies tracks what a post depends on
type Dependencies = models.Dependencies

// PostListMeta contains minimal metadata needed for navigation/sorting
type PostListMeta = models.PostListMeta

// CacheStats holds runtime statistics
type CacheStats = models.CacheStats

// CompressionType re-exports the compression type enum.
type CompressionType = models.CompressionType

const (
	// CompressionNone disables compression.
	CompressionNone = models.CompressionNone
	// CompressionZstdFast enables fast zstd compression.
	CompressionZstdFast = models.CompressionZstdFast
	// CompressionZstdLevel3 enables zstd compression at level 3.
	CompressionZstdLevel3 = models.CompressionZstdLevel3
)

const (
	// SchemaVersion is the current cache schema version.
	// This should be kept in sync with models.CurrentSchemaVersion (search index schema).
	// Both are currently at version 20.
	SchemaVersion = 21
)

// HashContent returns a hex xxh3 hash of the content.
func HashContent(data []byte) string {
	hash := xxh3.Hash128(data)
	hashBytes := hash.Bytes()
	return hex.EncodeToString(hashBytes[:])
}

// HashString returns a hex xxh3 hash of the string.
func HashString(inputString string) string {
	return HashContent([]byte(inputString))
}

// GeneratePostID derives a post ID from UUID or normalized path.
func GeneratePostID(uuid string, normalizedPath string) string {
	if uuid != "" {
		return HashString(uuid)
	}
	return HashString(normalizedPath)
}

// Encode marshals a msgp.Marshaler value.
func Encode(value any) ([]byte, error) {
	if marshaler, ok := value.(msgp.Marshaler); ok {
		return marshaler.MarshalMsg(nil)
	}
	return nil, fmt.Errorf("type %T does not implement msgp.Marshaler", value)
}

// Decode unmarshals data into a msgp.Unmarshaler value.
func Decode(data []byte, value any) error {
	if unmarshaler, ok := value.(msgp.Unmarshaler); ok {
		_, err := unmarshaler.UnmarshalMsg(data)
		return err
	}
	return fmt.Errorf("type %T does not implement msgp.Unmarshaler", value)
}
