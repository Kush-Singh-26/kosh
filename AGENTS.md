# Agentic Development Guide

This repository contains a custom Static Site Generator (SSG) built in Go, designed for high performance and agentic workflows. Use this guide to understand build processes, testing, and code conventions.

## 1. Build, Lint & Test

### Build Commands
The unified CLI tool `kosh` handles all operations.
*   **Build CLI:** `go build -o kosh.exe cmd/kosh/main.go`
*   **Build Site:** `./kosh.exe build` (Minifies HTML/CSS/JS, compresses images)
*   **Serve (Dev Mode):** `./kosh.exe serve --dev` (Starts server with live reload & watcher)
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
    *   **`run/`**: **Modularized.** Contains build orchestration logic split into `builder.go`, `build.go`, `incremental.go`, and specialized pipelines: `pipeline_assets.go`, `pipeline_posts.go`, `pipeline_meta.go`, `pipeline_pwa.go`, and `pipeline_pagination.go`. All pipelines now interact directly with `cache.Manager` (BoltDB), replacing the legacy in-memory `buildCache`.
    *   **`renderer/native/`**: Contains native D2 and LaTeX rendering logic, split into `renderer.go` (core), `math.go`, and `d2.go`.
    *   **`parser/`**: Markdown parsing logic (Goldmark extensions and modular AST transformers: `trans_url.go`, `trans_d2.go`, `trans_ssr.go`, `math.go`, `toc.go`).
    *   **`cache/`**: BoltDB-based cache system with content-addressed storage, compression, and garbage collection. Now also handles WASM source hashing and dependency tracking.
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
*   **Logging Guidelines:**
    *   **Dev Mode:** Keep output minimal - avoid progress bars, per-file status, and verbose emoji
    *   **Production:** Show essential info only (errors, final metrics)
    *   **Error Messages:** Always log errors with context, use `❌` prefix for visibility
    *   **Success Messages:** Keep minimal, avoid spam during watch mode
    *   **Progress:** Remove progress updates; use metrics summary instead
*   **Concurrency:**
    *   **Worker Pool:** A pool of `runtime.NumCPU()` workers handles rendering tasks in parallel.
    *   **Worker Sizing:** Adjust based on I/O vs CPU bound tasks. Use `runtime.NumCPU() * 3/2` for I/O-bound operations (cache reads).
    *   **Map Safety:** All shared map access is protected by `sync.Mutex`.
    *   **Goroutine Cleanup:** Use `sync.WaitGroup` to track pending goroutines and ensure clean shutdown.
*   **Caching & Incremental Builds:**
    *   **BoltDB Cache:** All metadata (Posts, Dependencies, Template Hashes) stored in `.kosh-cache/meta.db`. The legacy `buildCache` struct has been removed.
    *   **Content-Addressed Storage:** SSR artifacts (D2, KaTeX) stored in `.kosh-cache/store/` with BLAKE3 hashes.
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

## 3. Cursor/Agent Rules

*   **No Read-Only Mode:** Agents operate in build mode with full file system access.
*   **Proactive Fixes:** If you spot a lint error or logical bug while working on a feature, fix it.
*   **Verification:** After making changes:
    1.  Run `go build -o kosh.exe cmd/kosh/main.go` to ensure compilation.
    2.  Run `./kosh.exe build` to ensure the logic runs without runtime panics.
    3.  Run `golangci-lint run` to check for linting issues.
