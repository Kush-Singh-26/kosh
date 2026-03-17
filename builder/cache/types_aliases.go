package cache

import (
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
)

// Re-export core types for backward compatibility
type PostMeta = core.PostMeta
type SearchRecord = core.SearchRecord
type Dependencies = core.Dependencies
type PostListMeta = core.PostListMeta
type CacheStats = core.CacheStats

// Re-export GC types for backward compatibility
type GCConfig = gc.GCConfig
type GCResult = gc.GCResult

var DefaultGCConfig = gc.DefaultGCConfig

const (
	CompressionNone       = core.CompressionNone
	CompressionZstdFast   = core.CompressionZstdFast
	CompressionZstdLevel3 = core.CompressionZstdLevel3

	SchemaVersion = core.SchemaVersion
)

var HashContent = core.HashContent
var HashString = core.HashString
var GeneratePostID = core.GeneratePostID
var Encode = core.Encode
var Decode = core.Decode
