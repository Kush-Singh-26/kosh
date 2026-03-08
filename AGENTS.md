# Agentic Development Guide

This repository contains **Kosh**, a high-performance Static Site Generator (SSG) built in Go. This guide covers build processes, architecture, testing, and code conventions.

## Project Status: v1.3.9 ✅

All phases of development have been completed:
- **Phase 1**: Security & Stability (XXH3, graceful shutdown, error handling)
- **Phase 2**: Architecture Refactoring (Service Layer, Dependency Injection)
- **Phase 3**: Performance Optimization (Memory pools, pre-computed search)
- **Phase 4**: Modernization (Go 1.23, Generics, dependency updates)
- **Phase 5**: Search Enhancement (Msgpack, stemming, fuzzy search, phrase matching)
- **Phase 6**: Hugo-Style Distribution (detached themes, go install, custom outputDir)
- **Phase 7**: Performance Audit & Dead Code Cleanup (body hash caching, LRU cache, race condition fixes)
- **Phase 8**: libvips Integration (parallel image processing, 3x faster builds)
- **Phase 9**: Advanced Reliability & Memory Profiling (CGO leak fixes, thread-safe debounce, strict LRU)
- **Phase 10**: Deep Audit & Production Hardening (v1.3.0)
- **Phase 11**: Tier 2 Advanced Optimizations (v1.3.1)
- **Phase 12**: Tier 3 Final Production+ Polish (v1.3.2)
- **Phase 13**: Search Encoding Robustness (v1.3.5)
- **Phase 14**: Frontend Reliability (v1.3.6)
- **Phase 15**: Search Relevance & Live Match (v1.3.8)
- **Phase 16**: Search Snippet & Phrase Precision (v1.3.9)
    - **Content Restoration:** Re-added the `Content` field to the search index (Schema v6) to allow snippet extraction from the full body of the post rather than just the description.
    - **Implicit Phrase Boost:** Implemented automatic ranking boosts for multi-word queries that appear as exact phrases in the document, ensuring highly relevant posts appear at the top.
    - **Full-Body Highlighting:** Enabled the regex highlighter to scan the entire post body, ensuring terms mentioned deep in the text are correctly highlighted in results.

---

### v1.3.9 (2026-03-05)

**Search Snippet & Phrase Precision:**
- **Content Restoration**: Re-added `Content` to the index, enabling accurate snippets for any matched word in the post.
- **Implicit Phrase Boost**: Applied a 1.2x score boost for documents containing the exact query phrase.
- **Schema v6**: Upgraded indexing schema to support full-body text storage.

---

### v1.3.8 (2026-03-05)

**Search Relevance & Live Match:**
- **Prefix Matching**: Added support for matching partial words in the inverted index, improving the "as-you-type" experience.
- **Regex Highlighting**: Implemented a sophisticated regex highlighter that targets full words case-insensitively while preserving the document's original formatting.
- **Stop Word Fallback**: Ensured queries containing only stop words still return results by falling back to a direct metadata scan.

---

### v1.3.6 (2026-03-05)

**Frontend Reliability:**
- **Cache Busting**: Appended `?v={{ .BuildVersion }}` to `search.wasm`, `search.bin`, and `wasm_exec.js` fetches to prevent stale browser state.
- **Global Build Metadata**: Exposed `window.buildVersion` to allow JavaScript-driven fetches to remain synchronized with the SSG build version.

---

### v1.3.5 (2026-03-05)

**Search Encoding Robustness:**
- **Stringified IDs**: Switched search index maps (`DocLens`, `Inverted`) to use string keys, ensuring perfect cross-platform MessagePack compatibility between encoding (Go/Win64) and decoding (WASM/Browser).
- **Schema Validation**: Added explicit schema version checking in the browser to prevent stale cache issues.
- **WASM Reliability**: Improved error reporting and state management in the search client.

---

### v1.3.2 (2026-03-05)

**Final Production+ Polish:**
- **Density-Based Snippets**: Search results now highlight the 150-character window where query terms are most dense, significantly improving relevance.
- **WASM binary size**: Reduced `search.wasm` by ~200KB by stripping `fmt` and implementing lightweight Unicode build tags for the browser.
- **Incremental Social Cards**: Fixed a bug where social cards weren't refreshed in watch mode when metadata changed.
- **Zero-Waste Cache**: Purged redundant `Content` and `Tokens` fields from the BoltDB schema, resulting in 20% smaller cache files.
- **SSR Pre-allocation**: Improved D2 diagram and LaTeX math rendering speed by pre-allocating result slices.

---

### v1.3.1 (2026-03-05)

**Advanced Optimization Pass:**
- **Lazy Backups**: `TxSync` now only creates rollback backups for files that are actually being modified, eliminating thousands of redundant disk operations.
- **Smart Template Monitoring**: Replaced expensive content hashing with metadata-based monitoring for templates, significantly reducing I/O in watch mode.
- **Unified Search Structure**: Merged inverted index and positional maps into a single `word -> postID -> [positions]` map, reducing serialization overhead and payload size.
- **Search Content Removal**: Removed raw plain text from the search index, using the `Description` field for result snippets while keeping the full content searchable via the index.
- **Single-Pass AST**: Unified multiple Markdown tree traversals into a single pass, improving parsing speed.
- **AST Image Transformer**: Moved `.webp` transformation from a global regex pass to a specialized Goldmark AST transformer.
- **Parallel Parsing**: Core templates are now parsed concurrently using `errgroup`.
- **JS Bundling**: Enabled bundling for JavaScript to improve frontend performance.

---

### v1.3.0 (2026-03-05)

**Deep Audit & Production Hardening:**
- **Critical Deadlock Fixes**: Resolved deadlocks in server build-wait loops and worker pool task submission.
- **Neighbor Lookup Optimization**: Pre-indexed post positions within versions, reducing neighbor resolution from $O(N^2)$ to $O(N)$ for the full build.
- **Truly Incremental Search**: Implemented in-memory `IndexedPost` caching, allowing watch-mode rebuilds to update the search index in milliseconds without BoltDB re-reads.
- **Zero-Allocation Stemmer**: Optimized Porter stemmer to use direct rune-slice suffix checking, eliminating heavy string allocation pressure during indexing.
- **Single-Pass Snippet Highlighting**: Replaced multiple regex passes with a single `strings.Replacer` pass for all search terms.
- **Transactional VFS Sync**: Integrated `TxSync` properly into the build pipeline to allow atomic rollback of the entire physical output directory on sync failure.
- **Strict Size-Aware LRU**: Enhanced `imageCache` to strictly enforce memory limits (50MB) by evicting based on byte size rather than just item count.
- **Build Lock Enforcement**: Modified `kosh build` to fail by default if another instance is running, preventing concurrent output corruption (override via `--force-lock`).
- **Dynamic Server Pathing**: Development server now dynamically calculates request path normalization based on `baseURL` instead of using a hardcoded prefix.

---

### v1.2.3 (2026-03-01)

**Stability & Reliability Optimizations:**
- **Waitgroup Race Condition Fix**: Resolved a critical panic in `DiagramCacheAdapter` where async worker completion could outpace waitgroup incrementing.
- **CGO Memory Leak Fix**: Addressed massive memory leaks during image optimization by ensuring `libvips.ImageRef` instances are explicitly closed after processing.
- **Strict LRU Memory Cache**: Replaced standard Go map with `github.com/hashicorp/golang-lru/v2` to strictly enforce memory limits during long watch-mode sessions.
- **Search Index Integrity**: Fixed an issue where incremental builds corrupted BM25 data for live-edited files by integrating proper analysis into the single-post processing pipeline.
- **Ghost Drafts Fix**: Prevented drafts from appearing as broken 404 links in tags and pagination when drafts are excluded from the build.

**Server & Watcher Enhancements:**
- **Thread-safe Debounce**: Files modified rapidly in succession are now correctly batched and triggered together instead of dropping intermediary events.
- **SSE Broadcast Reliability**: Auto-reload events in watch mode are now buffered, preventing missed browser refreshes under high load.
- **Gzip Range Fix**: Custom `gzipHandler` now bypasses compression for `Range` requests, restoring scrub support for large media and PDF files.

**Dependencies:**
- Added `github.com/hashicorp/golang-lru/v2` for reliable memory management

---

### v1.2.2 (2026-02-17)

**Performance Optimizations:**
- **Parallel Index Building**: Search index now built in parallel using multiple goroutines for stem map generation, 2-4x faster for larger sites
- **LRU Memory Cache for Images**: In-memory LRU cache (50MB) added for processed images, significantly faster in watch mode with repeated builds
- **libvips Integration**: Switched from pure Go imaging libraries to libvips for 3-5x faster image processing
  - Parallel processing with configurable concurrency (`vipsConcurrency` config option)
  - Auto-detects CPU cores, defaults to min(CPU count, 4) threads
  - Increased cache: 100MB memory, 100 files, 100 operations

**Configuration:**
- Added `vipsConcurrency` option to `kosh.yaml`:
  ```yaml
  vipsConcurrency: 0  # Auto-detect (default: uses CPU count, capped at 4)
  vipsConcurrency: 4  # Use exactly 4 threads
  ```
- Suppressed verbose libvips logging (only warnings/errors shown)

**Dependencies:**
- Added `github.com/twincats/golibvips` v0.1.2 for high-performance image processing

---

### v1.2.1 (2026-02-16)

**Performance Optimizations:**
- **Body Hash Caching**: Body content hashed separately from frontmatter, fixing a critical bug where body-only changes were silently ignored by the cache
- **In-Memory LRU Cache**: Hot PostMeta data cached with 5-minute TTL, reducing BoltDB reads for frequently accessed posts
- **SSR Hash Tracking**: D2 diagrams and LaTeX math hashes now tracked in `SSRInputHashes` field for proper cache management
- **Stemming Cache**: `StemCached()` uses `sync.Map` for ~76x speedup on repeated words
- **Ngram Index for Fuzzy Search**: Pre-built trigram index enables ~20% faster fuzzy queries
- **Double ReadFile Fix**: Image encoding now done once to buffer, then written to both cache and destination

**Bug Fixes:**
- **Race Condition Fix**: Static assets now build synchronously before post rendering, ensuring `Assets` map is populated when templates render (previously caused CSS 404 errors on post pages)
- **filepath.WalkDir**: More efficient than `filepath.Walk`, avoids extra stat calls
- **bytes.Contains**: Avoids string allocation when checking frontmatter delimiters

**Dead Code Cleanup:**
- Removed unused breadcrumb functionality
- Removed unused pool instances (`SharedStringBuilderPool`, `SharedByteSlicePool`)
- Cleaned up empty test functions
- Removed duplicate code (favicon path helper, StoreHTML methods)

---

**Version:** v1.3.9  
**Last Updated:** 2026-03-05  
**Status:** Production Ready ✅
