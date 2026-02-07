package cache

// BoltDB bucket names
const (
	// Core buckets
	BucketPosts      = "posts"       // {PostID} -> PostMeta
	BucketPaths      = "paths"       // {filepath} -> PostID
	BucketSearch     = "search"      // {PostID} -> SearchRecord
	BucketPostDeps   = "post_deps"   // {PostID} -> Dependencies
	BucketSSR        = "ssr"         // {type}:{inputHash} -> SSRArtifact
	BucketSocialCard = "social_card" // {path} -> hash

	// Index buckets (set-based, value is empty)
	BucketTags          = "tags"           // {tag}/{PostID} -> empty
	BucketDepsTemplates = "deps_templates" // {template}/{PostID} -> empty
	BucketDepsIncludes  = "deps_includes"  // {include}/{PostID} -> empty

	// Global metadata
	BucketMeta  = "meta"  // schema_version, cache_id
	BucketStats = "stats" // last_gc, build_count, etc.

	// Meta keys
	KeySchemaVersion = "schema_version"
	KeyCacheID       = "cache_id"
	KeyLastGC        = "last_gc"
	KeyBuildCount    = "build_count"
	KeyGraphHash     = "graph_hash"
)

// AllBuckets returns all bucket names for initialization
func AllBuckets() []string {
	return []string{
		BucketPosts,
		BucketPaths,
		BucketSearch,
		BucketPostDeps,
		BucketSSR,
		BucketSocialCard,
		BucketTags,
		BucketDepsTemplates,
		BucketDepsIncludes,
		BucketMeta,
		BucketStats,
	}
}
