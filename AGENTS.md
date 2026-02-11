# Agentic Development Guide

This repository contains a custom Static Site Generator (SSG) built in Go, designed for high performance and agentic workflows. Use this guide to understand build processes, testing, and code conventions.

## 1. Build, Lint & Test

### Build Commands
The unified CLI tool `kosh` handles all operations.
*   **Build CLI:** `go build -o kosh.exe ./cmd/kosh`
*   **Build Site:** `./kosh.exe build` (Minifies HTML/CSS/JS, compresses images)
*   **Create Version:** `./kosh.exe version <name>` (Creates a frozen snapshot of documentation)
*   **Serve (Dev Mode):** `./kosh.exe serve --dev` (Starts server with live reload & watcher)
    *   **Note:** Dev mode skips PWA generation (manifest, service worker, icons) for faster builds
*   **Clean Output:** `./kosh.exe clean` (Cleans `public/` and triggers async background deletion)
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
*   **Logger Access:** Available via `Builder.logger` or direct `slog` package calls for utilities.
*   **Log Levels:** Info, Warn, Error
*   **Example:** `b.logger.Info("message", "key", value)` or `b.logger.Warn("message", "error", err)`
*   All major operations should include appropriate logging. Legacy `log.Printf` and `fmt.Printf` should be avoided in build pipelines.

### Build Metrics
Build performance is tracked via `builder/metrics/metrics.go`.
*   **Metrics Collected:** Build duration, cache hits/misses, posts processed.
*   **Output:** Minimal single-line format: `📊 Built N posts in Xs (cache: H/M hits, P%)`
*   **Dev Mode:** Metrics suppressed in `serve --dev` to reduce noise during watch mode.
*   **Usage:** Access via `Builder.metrics`.

### Project Structure
*   **`builder/`**: Core SSG logic (rendering, parsing, caching).
    *   **`run/`**: **Modularized.** Contains build orchestration logic split into:
        *   `builder.go` - Builder initialization and configuration.
        *   `build.go` - Main build orchestration.
        *   `incremental.go` - Watch mode and single-post fast rebuild logic.
        *   `pipeline_assets.go` - Asset processing (esbuild, images) with site-static support.
        *   `pipeline_posts.go` - Metadata collection and rendering with snapshot isolation.
        *   `pipeline_meta.go` - Configurable metadata generation (sitemap, RSS, graph).
        *   `pipeline_pwa.go` - PWA generation (manifest, SW, icons).
        *   `pipeline_pagination.go` - Pagination and tag rendering with global filtering.
    *   **`renderer/native/`**: Contains native D2 and LaTeX rendering logic (Server-Side Rendering).
    *   **`parser/`**: Markdown parsing logic (Goldmark extensions: **Admonitions**, `trans_url.go`, `trans_ssr.go`).
    *   **`cache/`**: BoltDB-based cache system with content-addressed storage and BLAKE3 hashing.
    *   **`search/`**: **Version-Aware Engine.** BM25 scoring with snippet extraction.
*   **`cmd/kosh/`**: Main entry point for the CLI.
*   **`cmd/search/`**: **WASM Bridge.** Compiles the search engine for browser execution.
*   **`content/`**: Markdown source files. Versioned folders are isolated snapshots.
*   **`themes/`**: **Themed Assets.**
    *   `blog/`: Default blog theme. Optimized for chronological feed.
    *   `docs/`: Documentation theme. Features a **Documentation Hub** and **Recursive Sidebar**.

### Documentation Theme (Docs)

The docs theme provides a professional documentation experience:

**Documentation Hub:**
- **Landing Page:** A high-level summary of all categories with a "Go to Latest" CTA.
- **Standalone 404:** A dedicated, styled error page for missing documentation.

**Versioning System:**
- **Snapshot Model:** Versions are independent folders (`content/v1.0/`).
- **Strict Navigation:** Next/Prev links and Sidebar items are version-scoped.
- **Version Banner:** Shows on outdated snapshots with a link to the latest version.

**Interactive Features:**
- **Search:** Version-scoped WASM search with snippets and keyboard navigation.
- **Mobile Nav:** Hamburger menu with slide-in sidebar.
- **Copy Code:** One-click copying for code blocks.
- **Theme Toggle:** Dark/light mode persistence with zero-flash implementation.

### Global SSG Features

- **Global Identity:** Site-wide logo and favicon configured via `logo` in `kosh.yaml`.
- **Parallel Sync:** VFS synchronization uses parallel worker pools for high-speed disk writes.
- **Cross-Platform Stability:** Absolute path resolution and Windows-Linux path normalization.
- **WASM Sync:** Search engine binary is embedded into the CLI and extracted during build.
    *   **Compile:** `GOOS=js GOARCH=wasm go build -o internal/build/wasm/search.wasm ./cmd/search`
    *   **Rebuild CLI:** `go build -ldflags="-s -w" -o kosh.exe ./cmd/kosh` (Required to embed new WASM).
