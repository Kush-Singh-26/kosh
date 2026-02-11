// Package cache provides a BoltDB + content-addressed filesystem cache for Kosh SSG.
// This implements compiler-grade incremental builds with crash-safe, deterministic state.
package cache

import (
	"encoding/hex"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zeebo/blake3"

	"my-ssg/builder/models"
)

// PostMeta stores metadata about a cached post
type PostMeta struct {
	PostID         string                 `msgpack:"post_id"`
	Path           string                 `msgpack:"path"`
	ModTime        int64                  `msgpack:"mod_time"`
	ContentHash    string                 `msgpack:"content_hash"`
	HTMLHash       string                 `msgpack:"html_hash,omitempty"`   // Only for large posts
	InlineHTML     []byte                 `msgpack:"inline_html,omitempty"` // < 32KB posts stored inline
	TemplateHash   string                 `msgpack:"template_hash"`
	SSRInputHashes []string               `msgpack:"ssr_input_hashes"`
	Title          string                 `msgpack:"title"`
	Date           time.Time              `msgpack:"date"`
	Tags           []string               `msgpack:"tags"`
	WordCount      int                    `msgpack:"word_count"`
	ReadingTime    int                    `msgpack:"reading_time"`
	Description    string                 `msgpack:"description"`
	Link           string                 `msgpack:"link"`
	Weight         int                    `msgpack:"weight"`
	Pinned         bool                   `msgpack:"pinned"`
	Draft          bool                   `msgpack:"draft"`
	Meta           map[string]interface{} `msgpack:"meta"`
	TOC            []models.TOCEntry      `msgpack:"toc"`
	Version        string                 `msgpack:"version"`
}

// Constants for inline HTML threshold
const (
	InlineHTMLThreshold = 32 * 1024 // 32KB - posts smaller than this are stored inline
)

// SSRArtifact stores server-side rendered content (D2 diagrams, KaTeX math)
type SSRArtifact struct {
	Type       string `msgpack:"type"`        // "d2", "katex"
	InputHash  string `msgpack:"input_hash"`  // BLAKE3 of input content
	OutputHash string `msgpack:"output_hash"` // BLAKE3 of output content (for store lookup)
	RefCount   int    `msgpack:"ref_count"`   // Advisory, derived during GC
	Size       int64  `msgpack:"size"`
	CreatedAt  int64  `msgpack:"created_at"`
	Compressed bool   `msgpack:"compressed"` // Whether output is zstd compressed
}

// SearchRecord stores pre-computed search data for BM25
type SearchRecord struct {
	Title    string         `msgpack:"title"`
	Tokens   []string       `msgpack:"tokens"`
	BM25Data map[string]int `msgpack:"bm25_data"` // word -> frequency
	DocLen   int            `msgpack:"doc_len"`
	Content  string         `msgpack:"content"`
	// Cached tokenization to avoid re-tokenizing unchanged content
	Words []string `msgpack:"words,omitempty"` // Cached tokenized words
}

// Dependencies tracks what a post depends on
type Dependencies struct {
	Templates []string `msgpack:"templates"`
	Includes  []string `msgpack:"includes"`
	Tags      []string `msgpack:"tags"`
}

// CacheStats holds runtime statistics
type CacheStats struct {
	TotalPosts    int   `msgpack:"total_posts"`
	TotalSSR      int   `msgpack:"total_ssr"`
	StoreBytes    int64 `msgpack:"store_bytes"`
	DeadBytes     int64 `msgpack:"dead_bytes"`
	LastGC        int64 `msgpack:"last_gc"`
	BuildCount    int   `msgpack:"build_count"`
	SchemaVersion int   `msgpack:"schema_version"`
	LastBuildTime int64 `msgpack:"last_build_time"`
	// Performance metrics for optimization monitoring
	LastReadTime  time.Duration `msgpack:"last_read_time"`
	LastWriteTime time.Duration `msgpack:"last_write_time"`
	ReadCount     int64         `msgpack:"read_count"`
	WriteCount    int64         `msgpack:"write_count"`
	InlinePosts   int           `msgpack:"inline_posts"` // Posts with inlined HTML
	HashedPosts   int           `msgpack:"hashed_posts"` // Posts using content-addressed storage
}

// CompressionType indicates how an artifact is stored
type CompressionType int

const (
	CompressionNone CompressionType = iota
	CompressionZstdFast
	CompressionZstdLevel3
)

// Constants for compression thresholds
const (
	RawThreshold  = 8 * 1024   // < 8KB stored raw
	FastZstdMax   = 128 * 1024 // 8KB-128KB use zstd fast
	SchemaVersion = 1
)

// HashContent computes BLAKE3 hash of content and returns hex string
func HashContent(data []byte) string {
	hash := blake3.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// HashString computes BLAKE3 hash of a string
func HashString(s string) string {
	return HashContent([]byte(s))
}

// GeneratePostID creates a stable PostID from UUID or normalized path
func GeneratePostID(uuid string, normalizedPath string) string {
	if uuid != "" {
		return HashString(uuid)
	}
	return HashString(normalizedPath)
}

// Encode serializes a value to msgpack bytes
func Encode(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

// Decode deserializes msgpack bytes to a value
func Decode(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}
