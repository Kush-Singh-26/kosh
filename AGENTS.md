# AGENTS.md

This file is the detailed working guide for coding agents operating in the Kosh repository.

## What Kosh Is

Kosh is a Go-based static site generator for blogs and documentation sites. It supports:

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

## Current Stable State

- Version string in CLI: `v1.3.9`
- Production-ready
- Dev-mode correctness issues recently fixed:
  - search schema mismatch in dev
  - stale asset-map lag on CSS changes
  - source-tree mutation from hardlink-based asset writes
  - incremental markdown rebuild path using absolute watcher paths
- Current recommended image settings:

```yaml
imageWorkers: 8
```

## Build Modes

### 1. Full Build

Entry path:

- `builder/run/build.go`

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

- `builder/run/incremental.go`

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
- `kosh version --info`

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

- `builder/run/build.go`
- `builder/run/incremental.go`
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

## Important Subsystems

### Asset Pipeline

Files:

- `builder/services/asset_service.go`
- `builder/utils/assets.go`
- `builder/utils/fs_copy.go`

Current behavior:

- esbuild handles CSS/JS and hashed assets
- root/static and theme/static files are copied via `CopyDirVFS`
- eligible local `.png/.jpg/.jpeg` are converted to `.webp`
- `search.wasm` from source static trees must not overwrite deployed runtime WASM
- logo/favicon may be exact-copied intentionally

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
- narrowed hash reuse was added, but reading-time reuse was intentionally deferred

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
- persisted D2 SSR cache is intentionally deferred / optional

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
- persisted D2 SSR cache

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

## Testing Guidance

### Fast targeted checks

```bash
go test ./builder/parser ./builder/services ./builder/run ./builder/utils
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
