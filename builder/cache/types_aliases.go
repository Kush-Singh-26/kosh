package cache

import (
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
)

// ContentMeta re-exports the core ContentMeta type for backward compatibility.
type ContentMeta = core.ContentMeta

// SearchRecord re-exports the core SearchRecord type for backward compatibility.
type SearchRecord = core.SearchRecord

// Dependencies re-exports the core Dependencies type for backward compatibility.
type Dependencies = core.Dependencies

// ContentListMeta re-exports the core ContentListMeta type for backward compatibility.
type ContentListMeta = core.ContentListMeta

// Stats re-exports the core CacheStats type for backward compatibility.
type Stats = core.CacheStats

// GCConfig re-exports the gc.Config type for backward compatibility.
type GCConfig = gc.Config

// GCResult re-exports the gc.Result type for backward compatibility.
type GCResult = gc.Result

// DefaultGCConfig re-exports the default GC configuration.
var DefaultGCConfig = gc.DefaultConfig

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

// GenerateContentID re-exports the content ID generator.
var GenerateContentID = core.GenerateContentID

// Encode re-exports the msgpack encoder.
var Encode = core.Encode

// Decode re-exports the msgpack decoder.
var Decode = core.Decode
