package core

// BoltDB bucket names
const (
	// Core buckets
	BucketPosts      = "posts"       // {PostID} -> PostMeta
	BucketPaths      = "paths"       // {filepath} -> PostID
	BucketSearch     = "search"      // {PostID} -> SearchRecord
	BucketPostDeps   = "post_deps"   // {PostID} -> Dependencies
	BucketSSR        = "ssr"         // {type}:{inputHash} -> SSRArtifact
	BucketSocialCard = "social_card" // {path} -> hash
	BucketFragments  = "fragments"   // {blockName:context:prefix} -> HTML

	// Index buckets (set-based, value is empty)
	BucketTaxonomies    = "taxonomies"     // {taxonomy}/{term}/{PostID} -> empty
	BucketDepsTemplates = "deps_templates" // {template}/{PostID} -> empty
	BucketDepsIncludes  = "deps_includes"  // {include}/{PostID} -> empty

	// Global metadata
	BucketMeta  = "meta"  // schema_version, cache_id
	BucketStats = "stats" // last_gc, build_count, etc.

	// Reference counting for content-addressed storage
	BucketRefCount = "ref_count" // {hash} -> count (for HTML blobs)

	// Meta keys
	KeySchemaVersion = "schema_version"
	KeyCacheID       = "cache_id"
	KeyLastGC        = "last_gc"
	KeyBuildCount    = "build_count"
	KeyGraphHash     = "graph_hash"
	KeyWasmHash      = "wasm_hash"
	KeySearchHash    = "search_hash"
)

// AllBuckets returns the list of BoltDB bucket names used by the cache.
func AllBuckets() []string {
	return []string{
		BucketPosts,
		BucketPaths,
		BucketSearch,
		BucketPostDeps,
		BucketSSR,
		BucketSocialCard,
		BucketTaxonomies,
		BucketDepsTemplates,
		BucketDepsIncludes,
		BucketMeta,
		BucketStats,
		BucketRefCount,
		BucketFragments,
	}
}
