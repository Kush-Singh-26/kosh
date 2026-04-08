package cache

import (
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
)

// PostMeta re-exports the core PostMeta type for backward compatibility.
type PostMeta = core.PostMeta

// SearchRecord re-exports the core SearchRecord type for backward compatibility.
type SearchRecord = core.SearchRecord

// Dependencies re-exports the core Dependencies type for backward compatibility.
type Dependencies = core.Dependencies

// PostListMeta re-exports the core PostListMeta type for backward compatibility.
type PostListMeta = core.PostListMeta

// CacheStats re-exports the core CacheStats type for backward compatibility.
type CacheStats = core.CacheStats

// GCConfig re-exports the gc.GCConfig type for backward compatibility.
type GCConfig = gc.GCConfig

// GCResult re-exports the gc.GCResult type for backward compatibility.
type GCResult = gc.GCResult

// DefaultGCConfig re-exports the default GC configuration.
var DefaultGCConfig = gc.DefaultGCConfig

const (
	// CompressionNone disables compression.
	CompressionNone = core.CompressionNone
	// CompressionZstdFast enables fast zstd compression.
	CompressionZstdFast = core.CompressionZstdFast
	// CompressionZstdLevel3 enables zstd compression at level 3.
	CompressionZstdLevel3 = core.CompressionZstdLevel3

	// SchemaVersion is the current cache schema version.
	SchemaVersion = core.SchemaVersion
)

// HashContent re-exports the content hashing function.
var HashContent = core.HashContent

// HashString re-exports the string hashing function.
var HashString = core.HashString

// GeneratePostID re-exports the post ID generator.
var GeneratePostID = core.GeneratePostID

// Encode re-exports the msgpack encoder.
var Encode = core.Encode

// Decode re-exports the msgpack decoder.
var Decode = core.Decode
