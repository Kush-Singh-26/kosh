# Agentic Development Guide

This repository contains **Kosh**, a high-performance Static Site Generator (SSG) built in Go. This guide covers build processes, architecture, testing, and code conventions for the completed v1.0.0 release.

## Project Status: v1.0.0 ✅

All four phases of development have been completed:
- **Phase 1**: Security & Stability (BLAKE3, graceful shutdown, error handling)
- **Phase 2**: Architecture Refactoring (Service Layer, Dependency Injection)
- **Phase 3**: Performance Optimization (Memory pools, pre-computed search)
- **Phase 4**: Modernization (Go 1.23, Generics, dependency updates)

---

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
*   **Show Version:** `./kosh.exe version` (Display version and optimization features)

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
    *   **`search/`**: **Version-Aware Engine.** BM25 scoring with pre-computed normalized fields.
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

## 8. Release Checklist

Before releasing a new version:

- [ ] All tests pass (`go test ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Binary builds successfully (`go build -o kosh.exe ./cmd/kosh`)
- [ ] Version command shows correct version (`./kosh version`)
- [ ] README.md is up to date
- [ ] AGENTS.md is up to date
- [ ] WASM is recompiled if search engine changed
- [ ] CHANGELOG.md is updated (if maintained)

---

**Version:** v1.0.0  
**Last Updated:** 2026-02-12  
**Status:** Production Ready ✅
