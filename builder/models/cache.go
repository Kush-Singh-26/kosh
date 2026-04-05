package models

//go:generate msgp

import (
	"time"
)

// PostMeta stores metadata about a cached post in BoltDB. It includes
// hashes for content and body to detect changes, and can inline small
// HTML content (<32KB) directly in the metadata bucket for O(1) retrieval.
type PostMeta struct {
	// PostID is a hex-encoded string (128-bit xxh3 hash) used as a unique
	// key in the cache. This is different from the decimal/uint64 ID used
	// in the search index.
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
	TOC             []TOCEntry
	CardHash        string
	HasImages       bool
	MathExpressions []MathExpression
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
	PositionalIndex map[string][]uint32
	ByteOffsets     map[string][]uint32
}

// Dependencies tracks what a post depends on
type Dependencies struct {
	Templates []string
	Includes  []string
	Tags      []string
}

// PostListMeta contains minimal metadata needed for navigation/sorting only.
// It is used to quickly build navigation and post lists
// without loading full PostMeta records from the cache.
type PostListMeta struct {
	Title  string
	Link   string
	Weight int
	Date   time.Time
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

// SSRArtifact stores server-side rendered content
type SSRArtifact struct {
	Type          string
	InputHash     string
	OutputHash    string
	RefCount      int
	Size          int64
	CreatedAt     int64
	Compressed    bool
	InlineContent []byte // Content < 16KB stored directly in BoltDB
}

type CompressionType int

const (
	CompressionNone CompressionType = iota
	CompressionZstdFast
	CompressionZstdLevel3
)
