// Package cache provides a BoltDB + content-addressed filesystem cache for Kosh SSG.
package core

//go:generate msgp

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/tinylib/msgp/msgp"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

var ErrNoContent = fmt.Errorf("no content found in cache")

// PostMeta stores metadata about a cached post
type PostMeta struct {
	PostID          string
	Path            string
	ModTime         int64
	ContentHash     string // Frontmatter hash
	BodyHash        string // Body content hash
	HTMLHash        string // Only for large posts
	InlineHTML      []byte // < 32KB posts stored inline
	SSRInputHashes  []string
	Title           string
	Date            time.Time
	Tags            []string
	WordCount       int
	ReadingTime     int
	Description     string
	Link            string
	Weight          int
	Pinned          bool
	Draft           bool
	Meta            map[string]any
	TOC             []models.TOCEntry
	Version         string
	CardHash        string
	HasImages       bool
	MathExpressions []models.MathExpression
}

// SSRArtifact stores server-side rendered content (D2 diagrams, KaTeX math)
// Type is stored as string for backward compatibility with existing cache data
type SSRArtifact struct {
	Type       string // "d2" or "math" - use models.SSRTypeD2/String() for conversion
	InputHash  string
	OutputHash string
	RefCount   int
	Size       int64
	CreatedAt  int64
	Compressed bool
}

// SearchRecord stores pre-computed search data for BM25
type SearchRecord struct {
	Title           string
	NormalizedTitle string
	BM25Data        map[string]int // word -> frequency
	DocLen          int
	Content         string
	NormalizedTags  []string
	StemMap         map[string]string
	PositionalIndex map[string][]int
	ByteOffsets     map[string][]int
}

// Dependencies tracks what a post depends on
type Dependencies struct {
	Templates []string
	Includes  []string
	Tags      []string
}

// PostListMeta contains minimal metadata needed for navigation/sorting
type PostListMeta struct {
	Title   string
	Link    string
	Weight  int
	Version string
	Date    time.Time
}

// CacheStats holds runtime statistics
type CacheStats struct {
	TotalPosts    int
	TotalSSR      int
	StoreBytes    int64
	LastGC        int64
	BuildCount    int
	SchemaVersion int
	InlinePosts   int
	HashedPosts   int
}

type CompressionType int

const (
	CompressionNone CompressionType = iota
	CompressionZstdFast
	CompressionZstdLevel3
)

const (
	SchemaVersion = 7
)

func HashContent(data []byte) string {
	hash := xxh3.Hash128(data)
	b := hash.Bytes()
	return hex.EncodeToString(b[:])
}

func HashString(s string) string {
	return HashContent([]byte(s))
}

func GeneratePostID(uuid string, normalizedPath string) string {
	if uuid != "" {
		return HashString(uuid)
	}
	return HashString(normalizedPath)
}

func Encode(v any) ([]byte, error) {
	if m, ok := v.(msgp.Marshaler); ok {
		return m.MarshalMsg(nil)
	}
	return nil, fmt.Errorf("type %T does not implement msgp.Marshaler", v)
}

func Decode(data []byte, v any) error {
	if u, ok := v.(msgp.Unmarshaler); ok {
		_, err := u.UnmarshalMsg(data)
		return err
	}
	return fmt.Errorf("type %T does not implement msgp.Unmarshaler", v)
}
