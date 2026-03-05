// Package cache provides a BoltDB + content-addressed filesystem cache for Kosh SSG.
package cache

import (
	"encoding/hex"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zeebo/blake3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// PostMeta stores metadata about a cached post
type PostMeta struct {
	PostID         string                 `msgpack:"post_id"`
	Path           string                 `msgpack:"path"`
	ModTime        int64                  `msgpack:"mod_time"`
	ContentHash    string                 `msgpack:"content_hash"`          // Frontmatter hash
	BodyHash       string                 `msgpack:"body_hash"`             // Body content hash
	HTMLHash       string                 `msgpack:"html_hash,omitempty"`   // Only for large posts
	InlineHTML     []byte                 `msgpack:"inline_html,omitempty"` // < 32KB posts stored inline
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

// SSRArtifact stores server-side rendered content (D2 diagrams, KaTeX math)
type SSRArtifact struct {
	Type       string `msgpack:"type"`
	InputHash  string `msgpack:"input_hash"`
	OutputHash string `msgpack:"output_hash"`
	RefCount   int    `msgpack:"ref_count"`
	Size       int64  `msgpack:"size"`
	CreatedAt  int64  `msgpack:"created_at"`
	Compressed bool   `msgpack:"compressed"`
}

// SearchRecord stores pre-computed search data for BM25
type SearchRecord struct {
	Title           string            `msgpack:"title"`
	NormalizedTitle string            `msgpack:"norm_title"`
	BM25Data        map[string]int    `msgpack:"bm25_data"` // word -> frequency
	DocLen          int               `msgpack:"doc_len"`
	Content         string            `msgpack:"content"`
	NormalizedTags  []string          `msgpack:"norm_tags"`
	StemMap         map[string]string `msgpack:"stem_map,omitempty"`
	PositionalIndex map[string][]int  `msgpack:"pos_index,omitempty"`
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
	LastGC        int64 `msgpack:"last_gc"`
	BuildCount    int   `msgpack:"build_count"`
	SchemaVersion int   `msgpack:"schema_version"`
	InlinePosts   int   `msgpack:"inline_posts"`
	HashedPosts   int   `msgpack:"hashed_posts"`
}

type CompressionType int

const (
	CompressionNone CompressionType = iota
	CompressionZstdFast
	CompressionZstdLevel3
)

const (
	SchemaVersion = 5
)

func HashContent(data []byte) string {
	hash := blake3.Sum256(data)
	return hex.EncodeToString(hash[:])
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

func Encode(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

func Decode(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}
