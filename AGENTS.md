# Agentic Development Guide

This repository contains a custom Static Site Generator (SSG) built in Go, designed for high performance and agentic workflows. Use this guide to understand build processes, testing, and code conventions.

## 1. Build, Lint & Test

### Build Commands
The unified CLI tool `kosh` handles all operations.
*   **Build CLI:** `go build -o kosh.exe ./cmd/kosh`
*   **Build Site:** `./kosh.exe build` (Minifies HTML/CSS/JS, compresses images)
*   **Serve (Dev Mode):** `./kosh.exe serve --dev` (Starts server with live reload & watcher)
    *   **Note:** Dev mode skips PWA generation (manifest, service worker, icons) for faster builds
*   **Clean Output:** `./kosh.exe clean` (Cleans `public/`)
*   **Clean All:** `./kosh.exe clean --cache` (Cleans `public/` and `.kosh-cache/`)

### Testing & Benchmarking
*   **Benchmark Suite:** `go test -bench=. -benchmem ./builder/benchmarks/`
    *   Benchmarks available: Search, Hash computation, Sorting, Tokenization, Snippet extraction
*   **Run All Tests:** `go test ./...`
*   **Run Single Test:** `go test ./path/to/pkg -run TestName -v`

### Linting
We use `golangci-lint` for static analysis.
*   **Run Linter:** `golangci-lint run`
*   **Fix Issues:** `golangci-lint run --fix`

## 2. Code Style & Conventions

### General Go
*   **Formatting:** Always use `gofmt` (handled by editor/IDE).
*   **Naming:**
    *   **Packages:** Short, lowercase, singular (e.g., `parser`, `config`).
    *   **Interfaces:** `er` suffix (e.g., `Renderer`, `Builder`).
    *   **Variables:** `camelCase`.
    *   **Exported:** `PascalCase`.
    *   **Constants:** Use const blocks with clear names, avoid magic numbers.
*   **Error Handling:**
    *   Check errors immediately: `if err != nil { return fmt.Errorf("context: %w", err) }`.
    *   Wrap errors with `%w` for context.
    *   Avoid `panic` unless startup fails critically.
    *   Log errors with structured logging (see below).

### Structured Logging
We use `log/slog` for structured logging throughout the codebase.
*   **Logger Access:** Available via `Builder.logger`
*   **Log Levels:** Info, Warn, Error
*   **Example:** `b.logger.Info("message", "key", value)` or `b.logger.Warn("message", "error", err)`
*   All major operations should include appropriate logging

### Build Metrics
Build performance is tracked via `builder/metrics/metrics.go`.
*   **Metrics Collected:** Build duration, cache hits/misses, posts processed
*   **Output:** Minimal single-line format: `📊 Built N posts in Xs (cache: H/M hits, P%)`
*   **Dev Mode:** Metrics suppressed in `serve --dev` to reduce noise during watch mode
*   **Usage:** Access via `Builder.metrics`
*   **Increment Methods:**
    *   `b.metrics.IncrementPostsProcessed()` - track posts
    *   `b.metrics.IncrementCacheHit()` - track cache usage
    *   `b.metrics.IncrementCacheMiss()` - track new content

### Project Structure
*   **`builder/`**: Core SSG logic (rendering, parsing, caching).
    *   **`run/`**: **Modularized.** Contains build orchestration logic split into:
        *   `builder.go` - Builder initialization and configuration
        *   `build.go` - Main build orchestration
        *   `incremental.go` - Incremental build logic and watch mode
        *   `pipeline_assets.go` - Asset processing (esbuild, images)
        *   `pipeline_posts.go` - Two-pass architecture (Collect -> Tree -> Render) with persistent social card cache. Support for `weight` and hierarchical `SiteTree`.
        *   `pipeline_meta.go` - Metadata generation (sitemap, RSS, graph)
        *   `pipeline_pwa.go` - PWA generation (manifest, SW, icons) with dev mode skip
        *   `pipeline_pagination.go` - Pagination and tag rendering with content hashing
        *   All pipelines now interact directly with `cache.Manager` (BoltDB), replacing the legacy in-memory `buildCache`.
    *   **`renderer/native/`**: Contains native D2 and LaTeX rendering logic, split into `renderer.go` (core), `math.go`, and `d2.go`.
    *   **`parser/`**: Markdown parsing logic (Goldmark extensions: **Admonitions**, `trans_url.go`, `trans_d2.go`, `trans_ssr.go`, `math.go`, `toc.go`).
    *   **`cache/`**: BoltDB-based cache system with content-addressed storage, compression, and garbage collection.
        *   `.kosh-cache/meta.db` - BoltDB database for post metadata, dependencies, template hashes
        *   `.kosh-cache/store/` - Content-addressed storage for SSR artifacts (D2, KaTeX)
        *   `.kosh-cache/images/` - WebP converted images
        *   `.kosh-cache/assets/` - esbuild results
        *   `.kosh-cache/social-cards/` - Generated social cards
        *   `.kosh-cache/pwa-icons/` - PWA icons (192x192, 512x512)
        *   Now also handles WASM source hashing and dependency tracking.
    *   **`benchmarks/`**: Performance benchmarks for critical paths (search, hashing, sorting).
    *   **`metrics/`**: Build performance tracking and telemetry.
*   **`cmd/kosh/`**: Main entry point for the CLI.
*   **`content/`**: Markdown source files.
*   **`themes/`**: **Themed Assets.**
    *   `blog/`: Default blog theme.
        *   `static/`: Theme-specific static assets (CSS, JS, Fonts). CSS is modularized in `static/css/components/`.
        *   `templates/`: HTML templates (`layout.html`, `index.html`, etc.).

### Imports
Group imports into three blocks separated by newlines:
1.  **Standard Library:** `fmt`, `os`, `strings`, etc.
2.  **Third-Party:** `github.com/yuin/goldmark`, `github.com/dop251/goja`, etc.
3.  **Internal:** `my-ssg/builder/...`

### Specific Logic Guidelines
*   **VFS Architecture:** All build operations write to a high-performance in-memory filesystem (`afero.MemMapFs`) first.
    *   **Differential Sync:** The state is synced atomically to the physical `public/` directory via `utils.SyncVFS`. It uses a "dirty file" tracking system (`Renderer.RegisterFile`) to only write modified files to disk.
    *   **Important:** Files required for functionality (like `search.bin`, `wasm_exec.js`) must be added to the `alwaysSync` list in `utils/sync.go`.
*   **Theme Engine:** The builder is designed to be theme-agnostic. Paths for templates and static assets are configurable via `kosh.yaml` (`templateDir`, `staticDir`).
*   **Asset Pipeline:** Uses `esbuild` Go API.
    *   **JS:** Processed with `Bundle: false` to preserve global library exports.
    *   **CSS:** Modular entry point `static/css/layout.css` bundles `core.css`, `theme.css`, and various components from `static/css/components/` via `@import`.
    *   **wasm_exec.js:** Copied separately in `pipeline_assets.go` since it shouldn't be processed by esbuild but is required for WASM search.
*   **Native Rendering (SSR):**
    *   **Lazy Initialization:** Rendering workers (Goja/KaTeX) are initialized lazily only when math or diagrams are detected in a build pass.
    *   **LaTeX:** Uses `github.com/dop251/goja` to run KaTeX JS server-side. Pre-rendered HTML is injected into the site.
    *   **D2 Diagrams:** Uses `oss.terrastruct.com/d2/d2lib`. Renders both light and dark themes as SVGs. Zoom logic in `main.js` handles theme-aware lightbox display.
    *   **Diagram Cache:** Uses `DiagramCacheAdapter` which requires `Close()` to be called for clean shutdown and goroutine cleanup. Uses bounded worker pool (NumCPU workers) to prevent goroutine explosion.
    *   **Font Caching:** Social card fonts loaded once and cached in memory across card generations.

*   **Social Card Generation:**
    *   **Post Cards:** Generated in `pipeline_posts.go` with content hashing. Uses direct-to-disk generation to avoid VFS overhead.
    *   **Home Card:** Generated in `pipeline_pagination.go` with hash based on site title + description.
    *   **Tag Cards:** Generated in `pipeline_pagination.go` with content-aware hashing (tag name + post count) to ensure updates when post count changes.
    *   **All Cards:** Cached in `.kosh-cache/social-cards/` and restored on clean builds.

*   **PWA Generation:**
    *   **Dev Mode Skip:** PWA generation (manifest, service worker, icons) is skipped entirely in `serve --dev` mode to speed up development.
    *   **Icon Caching:** PWA icons (192x192, 512x512) are cached in `.kosh-cache/pwa-icons/` based on favicon hash.
    *   **Source Path:** Icons are generated from `themes/<theme>/static/images/favicon.png`.
*   **Logging Guidelines:**
    *   **Dev Mode:** Keep output minimal - avoid progress bars, per-file status, and verbose emoji
    *   **Production:** Show essential info only (errors, final metrics)
    *   **Error Messages:** Always log errors with context, use `❌` prefix for visibility
    *   **Success Messages:** Keep minimal, avoid spam during watch mode
    *   **Progress:** Remove progress updates; use metrics summary instead

*   **Build Detection:**
    *   **Template-Only Changes:** Automatically detected and handled with fast path (re-renders from cache)
    *   **Content Changes:** Always trigger full processing to ensure updates are reflected
    *   **Frontmatter Changes:** Trigger social card regeneration and tag page updates
    *   **Config Changes:** (kosh.yaml) Force full rebuild including home page social card
*   **Concurrency:**
    *   **Worker Pool:** A pool of `runtime.NumCPU()` workers handles rendering tasks in parallel.
    *   **Worker Sizing:** Adjust based on I/O vs CPU bound tasks. Use `runtime.NumCPU() * 3/2` for I/O-bound operations (cache reads).
    *   **Map Safety:** All shared map access is protected by `sync.Mutex`.
    *   **Goroutine Cleanup:** Use `sync.WaitGroup` to track pending goroutines and ensure clean shutdown.
*   **Caching & Incremental Builds:**
    *   **BoltDB Cache:** All metadata (Posts, Dependencies, Template Hashes) stored in `.kosh-cache/meta.db`. The legacy `buildCache` struct has been removed.
    *   **Content-Addressed Storage:** SSR artifacts (D2, KaTeX) stored in `.kosh-cache/store/` with BLAKE3 hashes.
    *   **Persistent Image Cache:** WebP converted images cached in `.kosh-cache/images/` with source file hash keys.
    *   **Persistent Asset Cache:** esbuild results cached in `.kosh-cache/assets/` with content-addressed storage.
    *   **Social Card Caching:**
        *   Post cards: `.kosh-cache/social-cards/<frontmatter_hash>.webp`
        *   Home card: `.kosh-cache/social-cards/` (tracked via BoltDB hash)
        *   Tag cards: `.kosh-cache/social-cards/` with content-aware hashing (tag name + post count)
    *   **PWA Icon Caching:** Icons cached in `.kosh-cache/pwa-icons/<favicon_hash>-{192,512}.png`
    *   **Fast Rebuilds:** `incremental.go` queries BoltDB to determine invalidation. If only templates change, pages are reconstructed from cached PostMeta and HTML content.
    *   **Batch Cache Reads:** When reading multiple posts from cache, batch all BoltDB reads first in a single transaction, then parallelize rendering to avoid lock contention.
    *   **Batch Methods:** Use `GetPostsByIDs(ids []string)` and `GetSearchRecords(ids []string)` for N+1 query elimination.
    *   **WASM Hashing:** The search engine WASM source hash is persisted in BoltDB (`meta` bucket), enabling lazy compilation across restarts.
    *   **Search Index:** `SearchRecord` in BoltDB includes full text content to regenerate the search index without re-parsing Markdown.
    *   **Watcher:** Automatically tracks changes in `content/` and the active theme's template/static directories.
    *   **Cache Commands:** `kosh cache stats`, `kosh cache gc`, `kosh cache verify`, `kosh cache inspect <path>`.
*   **Template Caching:**
    *   Templates are cached globally with mtime tracking to avoid re-parsing on every build.
    *   Only re-parse when template files change.
    *   Cache is stored in `renderer.go` as a singleton.

### Performance Optimizations
The following optimizations have been implemented:

1. **Search Optimization:** Pre-allocated post cache in `search/engine.go` avoids repeated map lookups (40-60% faster).
2. **Hash Computation:** Direct hasher writes instead of JSON marshal (60% faster).
3. **Path Normalization:** Single-pass with `strings.Builder` (reduced allocations).
4. **Sorting:** Use `Unix()` timestamps instead of `time.Time` comparisons (10x faster).
5. **VFS Sync:** Size + mtime comparison before content hash (75% faster for unchanged files).
6. **Batch Processing:** Batch BoltDB reads to avoid lock contention (3-5x faster cached rebuilds).
7. **BatchCommit Pre-encoding:** Pre-encode data outside BoltDB transactions (3.75x faster writes).
8. **Inline Small HTML:** Store posts < 32KB inline in BoltDB, avoiding 2nd I/O (5x faster reads for small posts).
9. **BoltDB Tuning:** Dev mode uses `NoGrowSync=true` for 3x faster writes; production uses full durability.
10. **N+1 Query Fix:** Batch fetch posts by IDs in `GetPostsByIDs()` (50-70% faster template invalidation).
11. **sync.Pool:** Reuse `EncodedPost` slices during batch commits (20-30% less GC pressure).
12. **Bounded Workers:** Diagram cache uses fixed worker pool to prevent goroutine explosion.
13. **Parallel GC:** Store operations parallelized during garbage collection (2-3x faster GC).
14. **Tokenization Caching:** Cache tokenized words in `SearchRecord` to avoid re-tokenization.
15. **Lazy Native Renderer:** D2/KaTeX workers initialized only when diagrams/math are detected in content.
16. **Fast WASM Hash:** Uses file metadata (mtime+size) instead of content hashing for source change detection.
17. **Persistent Image Cache:** WebP processed images cached in `.kosh-cache/images/` to avoid re-encoding.
18. **Persistent Asset Cache:** esbuild results cached in `.kosh-cache/assets/` with content-addressed storage.
19. **Social Card Caching:** Home, tag index, and individual tag cards cached in `.kosh-cache/social-cards/`.
20. **PWA Icon Caching:** Icons (192x192, 512x512) cached in `.kosh-cache/pwa-icons/` with favicon hash tracking.
21. **Direct-to-Disk Generation:** Social cards generate directly to disk, avoiding VFS double-buffering.
22. **Font Caching:** Social card fonts loaded once and cached in memory across card generations.
23. **VFS Single-Read Pattern:** Small files read once for comparison and writing, reducing I/O by 50%.
24. **Skip Redundant Disk Checks:** On clean builds (known empty output), skip 39+ unnecessary `os.Stat` calls.
25. **Fixed Template-Only Detection:** Default to `false` to ensure content changes are always detected.
26. **Fixed Tag Card Updates:** Content-aware hashing (tag name + post count) ensures cards update when tags change.
28. **WASM Rebuild Fix:** Smart check for intermediate `search.wasm` in `static/` prevents 5s rebuild on `clean` command.
29. **KaTeX Pre-compilation:** Compiles JS script once and shares across workers, reducing init time from 3s to <300ms.
30. **Favicon Caching:** Decodes `favicon.png` only once (using `sync.Once`) and reuses image for 42+ social cards (saves ~3.6s).
31. **Parallel PWA Generation:** PWA icons/manifest generated concurrently with post processing (saves ~0.5s).
32. **Non-Blocking Worker Init:** Native renderer workers start immediately while others initialize in background, reducing startup latency.
33. **Optimized Background Clean:** `clean` command renames directories and deletes them in background to unblock build start.
34. **Two-Pass Architecture:** Separated metadata collection from rendering (`pipeline_posts.go`) to build global `SiteTree` for documentation sidebars without multiple disk reads.

### Build Performance (Updated)

| Build Scenario | Before | After | Improvement |
|---------------|--------|-------|-------------|
| **Clean Build** (`kosh clean && kosh build`) | ~18s | ~12.1s | **~33% faster** |
| **Cached Build** (`kosh build`) | ~1.2s | ~90-200ms | **6-12x faster** |
| **Clean --cache** (`clean --cache`) | ~18-20s | ~12.6s | **~35% faster** |
| **Dev Mode Incremental** | variable | ~100ms | **Instant** |

## 3. Cursor/Agent Rules

*   **No Read-Only Mode:** Agents operate in build mode with full file system access.
*   **Proactive Fixes:** If you spot a lint error or logical bug while working on a feature, fix it.
*   **Verification:** After making changes:
    1.  Run `go build -o kosh.exe cmd/kosh/main.go` to ensure compilation.
    2.  Run `./kosh.exe build` to ensure the logic runs without runtime panics.
    3.  Run `golangci-lint run` to check for linting issues.
