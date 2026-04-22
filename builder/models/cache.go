package models

//go:generate msgp

import (
	"time"
)

// ContentMeta stores metadata about a cached item in BoltDB. It includes
// hashes for frontmatter and body to detect changes, and can inline small
// HTML content (<32KB) directly in the metadata bucket for O(1) retrieval.
type ContentMeta struct {
	// ContentID is a hex-encoded string (128-bit xxh3 hash) used as a unique
	// key in the cache. This is different from the decimal/uint64 ID used
	// in the search index.
	ContentID      string
	Path           string
	ModTime        int64
	ContentHash    string // Frontmatter hash
	BodyHash       string // Body content hash
	HTMLHash       string // Only for large items
	InlineHTML     []byte // < 32KB items stored inline
	SSRInputHashes []string
	Section        string
	Title          string
	Date           time.Time
	Taxonomies     map[string][]string
	FileSize       int
	ReadingTime    int
	Description    string
	Link           string
	Weight         int
	IsPinned       bool
	IsHidden       bool
	IsDraft        bool
	// Meta stores raw frontmatter values for templating and downstream reuse.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	Meta            map[string]any
	TOC             []TOCEntry
	CardHash        string
	HasImages       bool
	MathExpressions []MathExpression
}

// SearchRecord stores pre-computed search data for BM25 (v2)
type SearchRecord struct {
	Title           string
	WordFreqs       map[string]int // word -> frequency
	DocLen          int
	Content         string
	Taxonomies      map[string][]string
	NormalizedTaxs  map[string][]string
	StemMap         map[string]string
	PositionalIndex map[string][]uint32
}

// Dependencies tracks what an item depends on
type Dependencies struct {
	Templates  []string
	Includes   []string
	Taxonomies map[string][]string
}

// ContentListMeta contains minimal metadata needed for navigation/sorting only.
// It is used to quickly build navigation and item lists
// without loading full ContentMeta records from the cache.
type ContentListMeta struct {
	Path       string
	Section    string
	Title      string
	Link       string
	Weight     int
	Date       time.Time
	Taxonomies map[string][]string
}

// CacheStats holds runtime statistics
type CacheStats struct {
	TotalItems    int
	TotalSSR      int
	StoreBytes    int64
	LastGC        int64
	BuildCount    int
	SchemaVersion int
	InlineItems   int
	HashedItems   int
}

// SSRArtifact stores server-side rendered content
type SSRArtifact struct {
	Type          string
	InputHash     string
	OutputHash    string
	RefCount      int
	Size          int64
	CreatedAt     int64
	IsCompressed  bool
	InlineContent []byte // Content < 16KB stored directly in BoltDB
}

// CompressionType describes compression levels for cached artifacts.
type CompressionType int

const (
	// CompressionNone disables compression.
	CompressionNone CompressionType = iota
	// CompressionZstdFast enables fast zstd compression.
	CompressionZstdFast
	// CompressionZstdLevel3 enables zstd compression at level 3.
	CompressionZstdLevel3
)
