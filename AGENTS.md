# Agentic Development Guide

This repository contains **Kosh**, a high-performance Static Site Generator (SSG) built in Go. This guide covers build processes, architecture, testing, and code conventions.

## Project Status: v1.2.0 ✅

All phases of development have been completed:
- **Phase 1**: Security & Stability (BLAKE3, graceful shutdown, error handling)
- **Phase 2**: Architecture Refactoring (Service Layer, Dependency Injection)
- **Phase 3**: Performance Optimization (Memory pools, pre-computed search)
- **Phase 4**: Modernization (Go 1.23, Generics, dependency updates)
- **Phase 5**: Search Enhancement (Msgpack, stemming, fuzzy search, phrase matching)
- **Phase 6**: Hugo-Style Distribution (detached themes, go install, custom outputDir)

---

## 1. Build, Lint & Test

### Installation

```bash
# Install via go install (recommended)
go install github.com/Kush-Singh-26/kosh/cmd/kosh@latest

# Verify installation
kosh version
```

### Build Commands
The unified CLI tool `kosh` handles all operations.
*   **Build CLI (development):** `go build -o kosh.exe ./cmd/kosh`
*   **Build Site:** `kosh build` (Minifies HTML/CSS/JS, compresses images)
*   **Create Version:** `kosh version <name>` (Creates a frozen snapshot of documentation)
*   **Serve (Dev Mode):** `kosh serve --dev` (Starts server with live reload & watcher)
    *   **Note:** Dev mode skips PWA generation (manifest, service worker, icons) for faster builds
    *   **Auto baseURL:** If `baseURL` is empty in config, dev mode auto-detects `http://localhost:2604`
*   **Clean Output:** `kosh clean` (Cleans root files only, preserves version folders)
*   **Clean All:** `kosh clean --all` (Cleans entire output directory including all versions)
*   **Clean Cache:** `kosh clean --cache` (Cleans root files and `.kosh-cache/`)
*   **Clean All + Cache:** `kosh clean --all --cache` (Cleans entire output and `.kosh-cache/`)
*   **Show Version:** `kosh version` (Display version and optimization features)

### CLI Commands Reference

| Command | Description |
|---------|-------------|
| `init [name]` | Initialize a new Kosh site |
| `new <title>` | Create a new blog post with the given title |
| `build` | Build the static site (and WASM search) |
| `serve` | Start the preview server |
| `clean` | Clean output directory |
| `cache <subcmd>` | Cache management commands |
| `version` | Version management commands |

### Build Flags

| Flag | Description |
|------|-------------|
| `--watch` | Watch for changes and rebuild automatically |
| `--cpuprofile <file>` | Write CPU profile to file (for profiling) |
| `--memprofile <file>` | Write memory profile to file (for profiling) |
| `-baseurl <url>` | Override base URL from config |
| `-drafts` | Include draft posts in build |
| `-theme <name>` | Override theme from config |

### Serve Flags

| Flag | Description |
|------|-------------|
| `--dev` | Enable development mode (build + watch + serve) |
| `--host <host>` | Host/IP to bind to (default: localhost) |
| `--port <port>` | Port to listen on (default: 2604) |
| `-drafts` | Include draft posts in development mode |
| `-baseurl <url>` | Override base URL from config |

### Clean Flags

| Flag | Description |
|------|-------------|
| `--cache` | Also clean `.kosh-cache/` directory |
| `--all` | Clean all versions including versioned folders |

**Note:** Clean command uses `outputDir` from config, supporting both relative and absolute paths.

### Cache Commands

| Command | Description |
|---------|-------------|
| `cache stats` | Show cache statistics and performance metrics |
| `cache gc` | Run garbage collection on cache |
| `cache verify` | Check cache integrity |
| `cache rebuild` | Clear cache for full rebuild |
| `cache clear` | Delete all cache data |
| `cache inspect <path>` | Show cache entry for a specific file |

### Cache GC Flags

| Flag | Description |
|------|-------------|
| `--dry-run`, `-n` | Show what would be deleted without deleting |

### Version Commands

| Command | Description |
|---------|-------------|
| `version` | Show current documentation version info |
| `version <vX.X>` | Freeze current latest and start new version |
| `version --info` | Show Kosh build information and optimizations |

### Testing & Benchmarking
*   **Benchmark Suite:** `go test -bench=. -benchmem ./builder/benchmarks/`
    *   Benchmarks available: Search, Hash computation, Sorting, Tokenization, Snippet extraction
*   **Run All Tests:** `go test ./...`
*   **Run Single Test:** `go test ./path/to/pkg -run TestName -v`

### Linting
We use `golangci-lint` for static analysis.
*   **Run Linter:** `golangci-lint run`
*   **Fix Issues:** `golangci-lint run --fix`

---

## 2. Architecture Overview

### Service Layer Pattern (Phase 2)

The codebase follows a clean architecture with separated concerns:

```
cmd/kosh/                    # CLI entry point
    └── main.go              # Command routing

builder/
├── services/                # Business Logic Layer
│   ├── interfaces.go        # Service contracts
│   ├── post_service.go      # Markdown parsing & indexing
│   ├── cache_service.go     # Thread-safe cache wrapper
│   ├── asset_service.go     # Static asset processing
│   └── render_service.go    # HTML template rendering
├── run/                     # Orchestration Layer
│   ├── builder.go           # Builder initialization (DI container)
│   ├── build.go             # Main build orchestration
│   └── incremental.go       # Watch mode & fast rebuilds
├── cache/                   # Data Access Layer
│   ├── cache.go             # BoltDB operations with generics
│   ├── types.go             # Data structures
│   └── adapter.go           # Diagram cache adapter
└── utils/                   # Utilities
    ├── pools.go             # Object pooling (BufferPool)
    └── worker_pool.go       # Generic worker pool
```

### Dependency Injection

The `Builder` struct acts as a composition root:

```go
type Builder struct {
    cacheService  services.CacheService
    postService   services.PostService
    assetService  services.AssetService
    renderService services.RenderService
    // ... other dependencies
}
```

Services are injected via constructors:
```go
func NewPostService(cfg *config.Config, cache CacheService, renderer RenderService, ...)
```

This enables:
- **Testability**: Easy mocking of dependencies
- **Separation of Concerns**: Each service has a single responsibility
- **Flexibility**: Swap implementations without changing business logic

---

## 3. Code Style & Conventions

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
    *   **Never ignore errors** - always handle or log them appropriately.

### Context & Cancellation
*   **Context Propagation:** All long-running operations must accept and respect `context.Context`.
*   **Graceful Shutdown:** The server and build operations support graceful shutdown via context cancellation.
*   **Signal Handling:** SIGINT (Ctrl+C) and SIGTERM trigger graceful shutdown with 5-second timeout.

### Security Best Practices (Phase 1)
*   **Path Validation:** All file paths are validated to prevent traversal attacks using `validatePath()`.
*   **Cryptographic Hashing:** Use BLAKE3 for all hashing operations (replaced deprecated MD5).
*   **Input Sanitization:** User-provided paths are normalized and validated before use.
*   **Safe Defaults:** Dev mode uses less durable but faster cache settings; production uses full durability.
*   **Case-Sensitive Paths:** `NormalizePath()` preserves case on Linux/macOS for case-sensitive filesystems (fixed in v1.2.0).

### Structured Logging
We use `log/slog` for structured logging throughout the codebase.
*   **Logger Access:** Available via `Builder.logger` or direct `slog` package calls for utilities.
*   **Log Levels:** Info, Warn, Error
*   **Example:** `b.logger.Info("message", "key", value)` or `b.logger.Warn("message", "error", err)`
*   All major operations should include appropriate logging. Legacy `log.Printf` and `fmt.Printf` should be avoided in build pipelines.

### Generics Usage (Phase 4)

We use Go 1.18+ generics for type-safe operations:

```go
// Generic cache retrieval
func getCachedItem[T any](db *bolt.DB, bucketName string, key []byte) (*T, error)

// Usage
post, err := getCachedItem[PostMeta](m.db, BucketPosts, []byte(postID))
```

Benefits:
- **Type Safety**: Compile-time type checking
- **Code Reduction**: Single implementation for all types
- **Performance**: No runtime type assertions

---

## 4. Performance Optimization Guidelines

### Memory Management (Phase 3)
We use object pooling to reduce GC pressure during high-throughput builds:
*   **BufferPool:** `builder/utils/pools.go` manages reusable `bytes.Buffer` instances for markdown rendering.
    ```go
    buf := utils.SharedBufferPool.Get()
    defer utils.SharedBufferPool.Put(buf)
    ```
*   **EncodedPostPool:** `builder/cache/cache.go` reuses slices for batch BoltDB commits.
*   **Strings.Builder:** Use for string concatenation instead of `+` operator.

### Worker Pools
Use the generic `WorkerPool[T]` for concurrent operations:
```go
pool := utils.NewWorkerPool(ctx, numWorkers, func(task MyTask) {
    // Process task
})
pool.Start()
// Submit tasks...
pool.Stop()
```

### Cache Optimization
*   **Inline Small Content**: Posts < 32KB store HTML inline in metadata (avoids 2nd I/O)
*   **Content-Addressed Storage**: Large content stored by BLAKE3 hash
*   **Batch Operations**: Group database writes for better throughput
*   **Pre-computed Fields**: Search indexes store normalized strings to avoid runtime `ToLower()`

### Build Metrics
Build performance is tracked via `builder/metrics/metrics.go`.
*   **Metrics Collected:** Build duration, cache hits/misses, posts processed.
*   **Output:** Minimal single-line format: `📊 Built N posts in Xs (cache: H/M hits, P%)`
*   **Dev Mode:** Metrics suppressed in `serve --dev` to reduce noise during watch mode.
*   **Usage:** Access via `Builder.metrics`.

---

## 5. Project Structure

### Core Packages
*   **`builder/`**: Core SSG logic (rendering, parsing, caching).
    *   **`services/`**: **Service Layer.** Decoupled logic for testability and injection.
        *   `interfaces.go` - Service contracts
        *   `post_service.go` - Markdown parsing, rendering, and indexing.
        *   `cache_service.go` - Thread-safe cache operations with sync.Map.
        *   `asset_service.go` - Static asset management.
        *   `render_service.go` - HTML template rendering wrapper.
    *   **`run/`**: **Orchestration.** Build coordination split into:
        *   `builder.go` - Builder initialization and DI wiring.
        *   `build.go` - Main build orchestration with context support.
        *   `incremental.go` - Watch mode and single-post fast rebuild logic.
        *   `pipeline_*.go` - Specialized pipelines (assets, posts, meta, PWA, pagination).
    *   **`renderer/native/`**: Native D2 and LaTeX rendering (Server-Side Rendering).
    *   **`parser/`**: Markdown parsing (Goldmark extensions: **Admonitions**, `trans_url.go`, `trans_ssr.go`).
    *   **`cache/`**: BoltDB-based cache with content-addressed storage and BLAKE3 hashing.
        *   Uses generic `getCachedItem[T any]` for type-safe retrieval
        *   Object pooling for batch operations
    *   **`search/`**: **Advanced Search Engine.** BM25 scoring with fuzzy matching, stemming, and phrase support.
        *   `engine.go` - Main search logic with BM25, fuzzy, and phrase matching
        *   `analyzer.go` - Text analysis pipeline (tokenization, stop words, stemming)
        *   `stemmer.go` - Porter stemmer implementation for English
        *   `fuzzy.go` - Levenshtein distance and fuzzy matching
*   **`cmd/kosh/`**: Main entry point for the CLI.
*   **`cmd/search/`**: **WASM Bridge.** Compiles the search engine for browser execution.
*   **`content/`**: Markdown source files. Versioned folders are isolated snapshots. (Removed in v1.2.0 - now separate from SSG)
*   **`themes/`**: **Detached in v1.2.0.** Themes are now separate repositories:
    *   `blog/`: Blog theme - `github.com/Kush-Singh-26/kosh-theme-blog`
    *   `docs/`: Documentation theme - `github.com/Kush-Singh-26/kosh-theme-docs`

### Documentation Theme (Docs)

The docs theme provides a professional documentation experience:

**Documentation Hub:**
- **Hub Page (`/`):** Template-only landing page (no `content/index.md` required) with "Go to Latest Docs" CTA
- **Version Cards:** Displays all available versions with "Current" badge on latest
- **Standalone 404:** A dedicated, styled error page for missing documentation

**Versioning System:**
- **Version Configuration:** Defined in `kosh.yaml` with `name`, `path`, and `isLatest` fields
- **Version Selector:** Dropdown shows `(Latest)` suffix for current version
- **Version Landing Pages:** Each version has its own `index.md` at `content/vX.X/index.md`
- **Version URL Preservation:** Switching versions preserves the current page path (e.g., `/getting-started.html` → `/v4.0/getting-started.html`)
- **Sparse Versioning:** Only changed pages need version-specific content; others fall back to latest
- **Outdated Banner:** Shows on non-latest versions with link to equivalent page in latest version
- **Fallback Handling:** If a page doesn't exist in the target version, redirects to version index or first available page

**URL Structure:**
```
/                           → Hub page (template-only)
/getting-started.html       → Latest version (dual access)
/v4.0/                      → Latest version landing page
/v4.0/getting-started.html  → Latest version (dual access)
/v3.0/                      → v3.0 landing page
/v3.0/quickstart.html       → v3.0 specific content
/v2.0/                      → v2.0 landing page
/v1.0/                      → v1.0 landing page
```

**Version URL Preservation:**
When switching versions via the dropdown selector, the current page path is preserved:
- On `/getting-started.html` → switch to v4.0 → `/v4.0/getting-started.html`
- On `/v3.0/quickstart.html` → switch to latest → `/quickstart.html`
- If the target page doesn't exist in that version, falls back to the first available page

The `GetVersionsMetadata(currentVersion, currentPath string)` function in `builder/config/config.go` handles URL generation with path preservation.

**Clean Command (Version-Aware):**
- `kosh clean` → Removes root files only, preserves version folders
- `kosh clean --all` → Removes entire output directory
- Uses config-based version detection to identify folders to preserve
- Handles both relative and absolute `outputDir` paths correctly

**Interactive Features:**
- **Search:** Version-scoped WASM search with snippets and keyboard navigation.
    - **BM25 Scoring:** Industry-standard relevance ranking
    - **Fuzzy Matching:** Typo tolerance (Levenshtein distance ≤ 2)
    - **Phrase Search:** Use quotes for exact phrases: `"machine learning"`
    - **Stemming:** Porter stemmer reduces words to roots ("running" → "run")
    - **Stop Words:** Common words filtered ("the", "and", "is", etc.)
    - **Msgpack Encoding:** ~30% smaller index, ~2.5x faster decode than GOB
- **Mobile Nav:** Hamburger menu with slide-in sidebar.
- **Copy Code:** One-click copying for code blocks.
- **Theme Toggle:** Dark/light mode persistence with zero-flash implementation.

### Global SSG Features

- **Global Identity:** Site-wide logo and favicon configured via `logo` in `kosh.yaml`.
- **Parallel Sync:** VFS synchronization uses parallel worker pools for high-speed disk writes.
- **Cross-Platform Stability:** Absolute path resolution and Windows-Linux path normalization.
- **WASM Search Engine:** Embedded into CLI and extracted during build.

### WASM Compilation

The search engine compiles to WebAssembly for browser-side execution. Recompile when modifying search logic.

**Full Compilation Process:**

| Step | Command | Purpose |
|------|---------|---------|
| 1 | `GOOS=js GOARCH=wasm go build -o internal/build/wasm/search.wasm ./cmd/search` | Compile WASM binary |
| 2 | `go build -ldflags="-s -w" -o kosh.exe ./cmd/kosh` | Rebuild CLI (embeds WASM) |
| 3 | `./kosh.exe clean --cache` | Clear old cached index |
| 4 | `./kosh.exe build` | Rebuild site with new format |

**PowerShell Equivalent:**
```powershell
# Step 1: Compile WASM
$env:GOOS="js"; $env:GOARCH="wasm"; go build -o internal/build/wasm/search.wasm ./cmd/search; Remove-Item Env:GOOS; Remove-Item Env:GOARCH

# Step 2: Rebuild CLI
go build -ldflags="-s -w" -o kosh.exe ./cmd/kosh

# Step 3-4: Clean and rebuild
.\kosh.exe clean --cache
.\kosh.exe build
```

**When to Recompile WASM:**
- ✅ `cmd/search/main.go` changes
- ✅ `builder/search/*.go` changes
- ✅ `builder/models/models.go` changes (SearchIndex struct)
- ✅ Serialization format changes (msgpack version)
- ❌ Content changes (no recompile needed)
- ❌ Theme changes (no recompile needed)

**Output Files:**
| File | Location | Size |
|------|----------|------|
| WASM source | `internal/build/wasm/search.wasm` | ~4.2 MB |
| Deployed WASM | `public/static/wasm/search.wasm` | Extracted from CLI |
| Search index | `public/search.bin` | ~200 KB (msgpack + gzip) |

---

## 6. Development Workflow

### Adding a New Feature

1. **Define Interface** (if needed): Add to `builder/services/interfaces.go`
2. **Implement Service**: Create or modify service in `builder/services/`
3. **Wire Dependencies**: Update `builder/run/builder.go` `NewBuilder()`
4. **Add Tests**: Write tests following existing patterns
5. **Update Documentation**: Update README.md and AGENTS.md

### Making Changes

1. **Small, atomic commits** for easier rollback
2. **Feature branches** with pull requests
3. **Mandatory code review** for all changes
4. **Run tests**: `go test ./...` before committing
5. **Run linter**: `golangci-lint run` before committing

### Testing Strategy

1. **Unit Tests**: Test individual functions and methods
2. **Integration Tests**: Test service interactions
3. **Performance Tests**: Benchmark before/after changes
4. **Race Detection**: `go test -race ./...`
5. **End-to-End**: Full build pipeline validation

---

## 7. Dependencies

### Current Versions (Phase 4)
*   **Go:** 1.23 (stable)
*   **Markdown:** `github.com/yuin/goldmark` v1.7.16
*   **Cache DB:** `go.etcd.io/bbolt` v1.4.3
*   **Hashing:** `github.com/zeebo/blake3` v0.2.4
*   **Compression:** `github.com/klauspost/compress` v1.18.4
*   **Serialization:** `github.com/vmihailenco/msgpack/v5` v5.4.1 (search index encoding)

### Updating Dependencies
```bash
go get -u ./...
go mod tidy
```

Verify build after updates:
```bash
go build -o kosh.exe ./cmd/kosh
go test ./...
```

---

## 8. Theme System (Detachable)

### Architecture

Themes are fully detachable and can be installed separately from the core SSG:

```
themes/
├── <theme-name>/
│   ├── templates/       # HTML templates (required)
│   │   ├── layout.html  # Base template
│   │   └── index.html   # Home page
│   ├── static/          # CSS, JS, images (optional)
│   └── theme.yaml       # Theme metadata
```

### Theme Configuration

In `kosh.yaml`:
```yaml
theme: "blog"           # Theme name
themeDir: "themes"      # Parent directory for themes
# templateDir: ""       # Override: defaults to themes/<theme>/templates
# staticDir: ""         # Override: defaults to themes/<theme>/static
```

### Installing Themes

```bash
# Clone a theme into the themes directory
git clone https://github.com/Kush-Singh-26/kosh-theme-blog themes/blog

# Or create a custom theme
mkdir -p themes/my-theme/templates themes/my-theme/static
```

### Creating Custom Themes

**Minimal `theme.yaml`:**
```yaml
name: "My Theme"
supportsVersioning: false  # Set true for docs-style versioned sites
```

**Required Templates:**
- `layout.html` - Base layout with `{{ template "content" . }}` block
- `index.html` - Home page template

### Theme Validation

The SSG validates theme presence at startup:
- Theme directory must exist at `themes/<theme-name>/`
- `templates/` directory is required
- `static/` directory is auto-created if missing

---

## 9. Custom Output Directory

Kosh supports custom output directories for Hugo-style integration with existing sites.

### Configuration

In `kosh.yaml`:
```yaml
outputDir: "../blogs"   # Output relative to project root
# or absolute path:
# outputDir: "/path/to/output"
```

### Use Case: Portfolio Integration

```
yourname.github.io/          # Portfolio repo
├── index.html               # Portfolio homepage
├── css/
├── js/
├── .github/workflows/
│   └── deploy.yml
└── blogs-src/               # Kosh project
    ├── kosh.yaml            # outputDir: "../blogs"
    ├── content/
    └── themes/
└── blogs/                   # Generated output
```

### Auto baseURL Detection

When `baseURL` is empty in config:
- **Dev mode** (`kosh serve --dev`): Auto-detects `http://localhost:2604`
- **Production**: Use `-baseurl` flag in CI

```bash
# Local development - no flags needed
kosh serve --dev

# Production build
kosh build -baseurl https://yourname.github.io/blogs
```

### URL Regeneration from Cache

Cached posts automatically regenerate URLs with the current `baseURL`. This allows:
- Build for production with one baseURL
- Build for local dev with different baseURL
- No cache invalidation needed when changing baseURL

---

## 9. Release Checklist

Before releasing a new version:

- [ ] All tests pass (`go test ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Binary builds successfully (`go build -o kosh.exe ./cmd/kosh`)
- [ ] Version command shows correct version (`./kosh version`)
- [ ] README.md is up to date
- [ ] AGENTS.md is up to date
- [ ] WASM recompiled if search engine changed (see Section 5 - WASM Compilation)
- [ ] CHANGELOG.md is updated (if maintained)

---

## 10. Search Engine Features

The search engine provides advanced full-text search capabilities:

### Architecture

```
builder/search/
├── engine.go       # Core search with BM25 scoring
├── analyzer.go     # Text processing pipeline
├── stemmer.go      # Porter stemmer for English
├── fuzzy.go        # Fuzzy matching and phrase parsing
└── search_test.go  # Comprehensive test suite
```

### Features

| Feature | Description | Example |
|---------|-------------|---------|
| **BM25 Scoring** | Industry-standard relevance ranking | Better results ordering |
| **Stemming** | Porter stemmer reduces words to roots | `running` → `run` |
| **Stop Words** | 115+ common English words filtered | `the`, `and`, `is` ignored |
| **Fuzzy Search** | Typo tolerance with Levenshtein distance | `trnsformer` → `transformer` |
| **Phrase Matching** | Exact phrase search with quotes | `"machine learning"` |
| **Trigram Index** | Fast fuzzy candidate lookup | Efficient typo correction |

### Encoding

- **Format:** Msgpack + gzip
- **Size Reduction:** ~30% smaller than GOB
- **Decode Speed:** ~2.5x faster than GOB
- **Cross-Platform:** Msgpack is language-agnostic

### Search Query Syntax

```
machine learning        # Terms search (stemmed)
"machine learning"      # Phrase search (exact)
tag:transformer         # Tag filter
tag:nlp attention       # Tag + terms
```

---

**Version:** v1.2.0  
**Last Updated:** 2026-02-15  
**Status:** Production Ready ✅
