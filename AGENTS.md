# AGENTS.md

This file is the detailed working guide for coding agents operating in the Kosh repository.

## What Kosh Is

Kosh is a Go-based static site generator for blogs. It supports:

- full static builds
- incremental watch-mode rebuilds
- CSS/JS bundling with hashed assets
- WebP image conversion for eligible raster assets
- BoltDB-backed caching
- server-side rendering for LaTeX and D2
- Go+WASM search
- RSS, sitemap, graph, PWA, and social card generation

Repository root:

- `C:\Users\KIIT0001\blogs`

Typical consumer site repo example used during development:

- `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`

## Theme Development Layout (Junctions)

Theme work is centralized in the Kosh submodule path:

- Canonical theme path: `C:\Users\KIIT0001\blogs\themes\blog`

Two convenience junctions point to the same files for local testing:

- `C:\Users\KIIT0001\kosh-theme-blog` (junction)
- `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src\themes\blog` (junction)

Rules for agents:

1. Treat `C:\Users\KIIT0001\blogs\themes\blog` as the source of truth.
2. Commit and push theme changes from the canonical path only.
3. Avoid editing any `*.bak` backup folders.

## Current Stable State

- Version string in CLI: `v1.4.0`
- Production-ready
- Dev-mode correctness issues recently fixed:
  - search schema mismatch in dev
  - stale asset-map lag on CSS changes
  - source-tree mutation from hardlink-based asset writes
  - incremental markdown rebuild path using absolute watcher paths
  - persistent SSR cache for diagrams and math
  - reading-time reuse during frontmatter-only updates
  - automatic cache garbage collection
- New features implemented:
  - Pipelined parse+render with streaming workers
  - Overlapping asset discovery and copying via channels
  - Decoupled search analysis via background worker pool
  - SEO suite: JSON-LD structured data + robots.txt generator
  - SVG minification via tdewolff/minify
  - A11y linting: build-time warnings for missing image alt text
- Current recommended image settings:

```yaml
imageWorkers: 8
```

## Build Modes

### 1. Full Build

Entry path:

- `builder/orchestration/build.go`

This path is used by:

- `kosh build`
- `kosh clean`
- `kosh clean --cache`
- initial `kosh serve --dev`

High-level phases:

1. metadata scan
2. asset build/copy
3. markdown parse and page render
4. site-wide generators
5. publish transaction commit

### 2. Incremental Dev Rebuild

Entry path:

- `builder/orchestration/incremental.go`

Important behavior:

- body-only markdown changes should use true single-post rebuild
- CSS/JS changes should rebuild assets and rerender HTML with fresh asset hashes
- search WASM should rebuild only if search source changed

## Commands and Behavior

### Main CLI

- `kosh build`
- `kosh serve --dev`
- `kosh clean`
- `kosh clean --cache`
- `kosh new "Title"`
- `kosh version`

### Important command semantics

- `kosh clean`
  - removes output and rebuilds
  - preserves `.kosh-cache`
- `kosh clean --cache`
  - removes output and `.kosh-cache`
  - forces a fully cold rebuild
- `kosh serve --dev`
  - build + watch + serve
  - should not mutate source files
  - should keep `window.buildVersion` fresh on rebuilds

## Key Architectural Rules

### 1. Source Tree Must Never Be Mutated

This is a hard rule.

Never write into:

- site `content/`
- site `static/`
- site theme assets/templates

All build outputs must go through:

- `builder/utils/transaction.go`
- `builder/utils/sink.go`

Important related protections:

- source→output hardlinking was removed from asset copy paths
- sink writes are restricted to staging/output roots

### 2. Clean Build Publishing Must Be Atomic

Clean builds use staging directories and publish via rename/swap.

Key file:

- `builder/utils/transaction.go`

Current state:

- unique staging/backup dirs for clean builds
- stale temp dir cleanup on startup
- retry/jitter around Windows rename operations

Do not break:

- atomic publish guarantees
- rollback behavior on publish failure

### 3. Asset Hashes Must Match Rendered HTML

CSS/JS changes must never leave HTML one build behind.

Important files:

- `builder/orchestration/build.go`
- `builder/orchestration/incremental.go`
- `builder/services/asset_service.go`
- `builder/services/render_service.go`

Important rule:

- if hashed assets change, pages referencing them must rerender with the new asset map

### 4. Search Runtime and Search Index Schema Must Match

Important files:

- `cmd/search/main.go`
- `builder/models/models.go`
- `themes/blog/static/js/search.js`
- `internal/build/build.go`

Critical invariant:

- `search.wasm`, `search.bin`, and browser bootstrap must all be in sync

### 5. Clean Builds vs Dev Builds Are Different

Do not optimize one in a way that breaks the other.

- clean/full builds care about staging correctness and reproducibility
- dev builds care about incremental speed and live correctness

## Building Kosh

### From Source

```bash
# Standard build
go build -o kosh ./cmd/kosh

# Install to $GOPATH/bin
go install ./cmd/kosh

# Cross-compilation
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o search.wasm ./cmd/search
```

### Prerequisites

- Go 1.26 or later
- Git

### Running Tests

```bash
# All tests
go test ./...

# Targeted tests
go test ./builder/parser ./builder/services ./builder/orchestration ./builder/utils

# With clean verification
go test ./builder/utils ./builder/services ./builder/run ./internal/clean
```

### Linting

```bash
golangci-lint run ./...
```

## Search WASM Development

### When to Rebuild

Rebuild the search WASM when:
- Search logic changes in `builder/search/`
- Schema version changes in `builder/models/models.go`
- Adding new search features

### Rebuild Commands

```bash
# Using Go run (for development)
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o search.wasm ./cmd/search

# The output will be in the current directory
```

### How It Works

1. **Embedded by default**: The WASM is pre-compiled and embedded in `internal/build/build.go`
2. **Schema versioning**: `models.CurrentSchemaVersion` must match between build and runtime
3. **Hash comparison**: Uses xxh3 to avoid unnecessary redeployments
4. **Caching**: Source-built WASM is cached in `.kosh-cache/wasm/<hash>.br`

### Important Files

- `cmd/search/main.go` - WASM entry point
- `builder/search/` - Search algorithm implementation
- `builder/models/models.go` - Schema version definition
- `internal/build/build.go` - WASM embedding and deployment

## Cache & Schema Versioning

### Schema Version Alignment

**Important:** Cache schema version (`core.SchemaVersion`) and search schema version (`models.CurrentSchemaVersion`) are now aligned at version 10 (as of 2026-03-19).

When making changes that affect serialized data structures:
1. Update `models.CurrentSchemaVersion` in `builder/models/models.go`
2. Update `core.SchemaVersion` in `builder/cache/core/types.go` to match
3. Add appropriate migrations in `builder/cache/migrate/migrations.go` if needed
4. Update search WASM if search schema changes

### Cache Schema

Files:
- `builder/cache/core/types.go` - Cache types and schema version
- `builder/cache/migrate/migrations.go` - Cache migrations
- `builder/models/cache.go` - Cache stats and types

Current schema version: 10

### RepoRoot Implementation

- `builder/fs/path.go` provides `RepoRoot()` and `RepoPath()` functions.
- **Important**: Standard users (installed binary) should NEVER rely on `RepoRoot()` as it resolves to `.` (cwd). Source compilation features are only for developers.
- Developers: If you must modify paths, prefer explicit configuration over magic path traversal.
- **Configuration**: Use `KOSH_REPO_ROOT` environment variable to override root detection.

## Important Subsystems

### Asset Pipeline

Files:

- `builder/services/asset_service.go`
- `builder/assets/pipeline.go`
- `builder/assets/image_processing.go`
- `builder/assets/image_cache.go`
- `builder/minify/html.go`
- `builder/fs/fs_copy.go` (low-level only)

Current behavior:

- esbuild handles CSS/JS and hashed assets
- root/static and theme/static files are copied via `CopyDirVFS`
- eligible local `.png/.jpg/.jpeg` are converted to `.webp` and originals removed from output
- `CleanupOriginalImages` removes source raster files when `.webp` equivalents exist
- `search.wasm` from source static trees must not overwrite deployed runtime WASM
- `favicon.png`, `icon-192.png`, `icon-512.png` are always kept as `.png` (critical for browser/PWA)

Current bottleneck for cold builds:

- `Asset copy root/static`

### Markdown Parse / Post Processing

Files:

- `builder/services/scanner.go`
- `builder/services/post_service.go`
- `builder/services/post_parser.go`
- `builder/parser/*`

Current notes:

- scanner performs lightweight frontmatter extraction
- parser is still the source of truth for full semantic parse
- narrowed hash reuse is used for reading-time and data reuse during frontmatter-only updates

### Search

Files:

- `builder/search/*`
- `builder/generators/search.go`
- `cmd/search/main.go`

Current notes:

- schema validation exists and browser errors if stale
- search snippets rely on stored content and offsets
- search-analysis restructuring remains deferred because it is high risk

### Math and D2 SSR

Files:

- `builder/parser/math.go`
- `builder/parser/trans_ssr.go`
- `builder/renderer/native/*`

Current notes:

- math SSR was narrowed to a single-scan match-list reuse approach
- persistent D2 and Math SSR cache is active in BoltDB

## Performance State

### Warm Full Builds (`kosh clean`)

Current state is good after recent fixes.

### Cold Full Builds (`kosh clean --cache`)

Still dominated by:

- `Asset copy root/static`
- `Parse 39 posts`

This is expected because caches are removed.

### Benchmarked Best Config

From the benchmark matrix in the site repo:

- best overall stable config:

```yaml
imageWorkers: 8
```

- this should remain the default recommendation in docs unless newer benchmark data proves otherwise

## Recently Fixed Issues Agents Should Know About

### Fixed

- source static corruption caused by hardlink-based output writes
- stale dev search WASM / schema mismatch
- CSS changes lagging one build behind due to stale asset map reuse
- font decode failures caused by zero-byte source fonts in site repo
- absolute watcher path mismatch for markdown incremental rebuilds
- Windows clean-build publish rename failures from fixed temp dir reuse

### Still intentionally deferred / optional

- search-analysis restructure

## Safe Change Boundaries

### Safe / low-risk

- docs updates
- CLI help text improvements
- local AST pass cleanup when behavior is already duplicated elsewhere
- duplicate write removal
- Windows retry/backoff hardening

### Medium risk

- scanner/parser data reuse
- asset-pipeline scheduling changes
- math SSR refactors

### High risk

- search analysis / ranking / snippet changes
- schema changes
- anything that alters clean-build staging completeness

## Things Agents Must Not Regress

- no source-tree writes
- no stale asset hashes in rendered HTML
- no stale search runtime/index mismatch
- no loss of incremental single-post rebuild for body-only content edits
- no clean-build publish partial-output state
- no broken `.webp` link rewriting for eligible local raster images
- no original raster images (.png/.jpg/.jpeg) left in output when .webp exists (except favicon.png, icon-192.png, icon-512.png)

## Testing Guidance

### Fast targeted checks

```bash
go test ./builder/parser ./builder/services ./builder/orchestration ./builder/utils
go test ./builder/utils ./builder/services ./builder/run ./internal/clean
```

### Real-world verification against site repo

From `blogs-src`:

```powershell
kosh clean --cache
kosh serve --dev
```

Verify:

- search works
- CSS updates apply immediately
- body-only markdown edits use incremental path
- no source mutations in `content/` or `static/`

### Hash-based source immutability check

Use before/after file-hash snapshots on:

- `static/`
- `content/`

## Benchmarking Guidance

Site-side helper files:

- `docs/build-benchmark-results.md`
- `docs/build-benchmark-results-warm.md`
- `scripts/benchmark-clean-warm.ps1`
- `scripts/benchmark-clean-cache.ps1`

Current recommendation after benchmarking:

```yaml
imageWorkers: 8
```

## Documentation Expectations

When updating docs, keep these accurate:

- `clean` rebuilds immediately after cleaning
- `clean --cache` is a true cold rebuild
- best-known image settings are `8`
- eligible raster image formats are `.png/.jpg/.jpeg` -> `.webp`
- warm builds are fast; cold builds are dominated by real image work

## Final Guidance For Agents

When in doubt:

1. protect correctness over speed
2. protect source immutability over convenience
3. keep search/schema/runtime in sync
4. avoid broad refactors in search unless fully tested
5. prefer small safe optimizations over large coupled rewrites
