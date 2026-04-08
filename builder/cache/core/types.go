// Package cache provides a BoltDB + content-addressed filesystem cache for Kosh SSG.
package core

import (
	"encoding/hex"
	"fmt"

	"github.com/tinylib/msgp/msgp"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// ErrNoContent indicates a cache miss when reading content.
var ErrNoContent = fmt.Errorf("no content found in cache")

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
	// Both are currently at version 12.
	SchemaVersion = 12
)

// HashContent returns a hex xxh3 hash of the content.
func HashContent(data []byte) string {
	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

// HashString returns a hex xxh3 hash of the string.
func HashString(s string) string {
	return HashContent([]byte(s))
}

// GeneratePostID derives a post ID from UUID or normalized path.
func GeneratePostID(uuid string, normalizedPath string) string {
	if uuid != "" {
		return HashString(uuid)
	}
	return HashString(normalizedPath)
}

// Encode marshals a msgp.Marshaler value.
func Encode(v any) ([]byte, error) {
	if m, ok := v.(msgp.Marshaler); ok {
		return m.MarshalMsg(nil)
	}
	return nil, fmt.Errorf("type %T does not implement msgp.Marshaler", v)
}

// Decode unmarshals data into a msgp.Unmarshaler value.
func Decode(data []byte, v any) error {
	if u, ok := v.(msgp.Unmarshaler); ok {
		_, err := u.UnmarshalMsg(data)
		return err
	}
	return fmt.Errorf("type %T does not implement msgp.Unmarshaler", v)
}
