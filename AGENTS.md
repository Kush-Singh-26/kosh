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

### Testing
There are currently no Go test files (`*_test.go`) detected in the repository.
*   **Future Tests:** When writing tests, place them alongside the source code (e.g., `builder/parser/parser_test.go`).
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
*   **Error Handling:**
    *   Check errors immediately: `if err != nil { return fmt.Errorf("context: %w", err) }`.
    *   Wrap errors with `%w` for context.
    *   Avoid `panic` unless startup fails critically.

### Project Structure
*   **`builder/`**: Core SSG logic (rendering, parsing, caching).
    *   **`run/`**: **Modularized.** Contains build orchestration logic split into `builder.go`, `build.go`, `incremental.go`, and specialized pipelines: `pipeline_assets.go`, `pipeline_posts.go`, `pipeline_meta.go`, `pipeline_pwa.go`, and `pipeline_pagination.go`. All pipelines now interact directly with `cache.Manager` (BoltDB), replacing the legacy in-memory `buildCache`.
    *   **`renderer/native/`**: Contains native D2 and LaTeX rendering logic, split into `renderer.go` (core), `math.go`, and `d2.go`.
    *   **`parser/`**: Markdown parsing logic (Goldmark extensions and modular AST transformers: `trans_url.go`, `trans_d2.go`, `trans_ssr.go`, `math.go`, `toc.go`).
    *   **`cache/`**: BoltDB-based cache system with content-addressed storage, compression, and garbage collection. Now also handles WASM source hashing and dependency tracking.
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
*   **Theme Engine:** The builder is designed to be theme-agnostic. Paths for templates and static assets are configurable via `kosh.yaml` (`templateDir`, `staticDir`).
*   **Asset Pipeline:** Uses `esbuild` Go API.
    *   **JS:** Processed with `Bundle: false` to preserve global library exports.
    *   **CSS:** Modular entry point `static/css/layout.css` bundles `core.css`, `theme.css`, and various components from `static/css/components/` via `@import`.
*   **Native Rendering (SSR):**
    *   **Lazy Initialization:** Rendering workers (Goja/KaTeX) are initialized lazily only when math or diagrams are detected in a build pass.
    *   **LaTeX:** Uses `github.com/dop251/goja` to run KaTeX JS server-side. Pre-rendered HTML is injected into the site.
    *   **D2 Diagrams:** Uses `oss.terrastruct.com/d2/d2lib`. Renders both light and dark themes as SVGs. Zoom logic in `main.js` handles theme-aware lightbox display.
*   **Concurrency:**
    *   **Worker Pool:** A pool of `runtime.NumCPU()` workers handles rendering tasks in parallel.
    *   **Map Safety:** All shared map access is protected by `sync.Mutex`.
*   **Caching & Incremental Builds:**
    *   **BoltDB Cache:** All metadata (Posts, Dependencies, Template Hashes) stored in `.kosh-cache/meta.db`. The legacy `buildCache` struct has been removed.
    *   **Content-Addressed Storage:** SSR artifacts (D2, KaTeX) stored in `.kosh-cache/store/` with BLAKE3 hashes.
    *   **Fast Rebuilds:** `incremental.go` queries BoltDB to determine invalidation. If only templates change, pages are reconstructed from cached PostMeta and HTML content.
    *   **WASM Hashing:** The search engine WASM source hash is persisted in BoltDB (`meta` bucket), enabling lazy compilation across restarts.
    *   **Search Index:** `SearchRecord` in BoltDB includes full text content to regenerate the search index without re-parsing Markdown.
    *   **Watcher:** Automatically tracks changes in `content/` and the active theme's template/static directories.
    *   **Cache Commands:** `kosh cache stats`, `kosh cache gc`, `kosh cache verify`, `kosh cache inspect <path>`.

## 3. Cursor/Agent Rules

*   **No Read-Only Mode:** Agents operate in build mode with full file system access.
*   **Proactive Fixes:** If you spot a lint error or logical bug while working on a feature, fix it.
*   **Verification:** After making changes:
    1.  Run `go build -o kosh.exe cmd/kosh/main.go` to ensure compilation.
    2.  Run `./kosh.exe build` to ensure the logic runs without runtime panics.
