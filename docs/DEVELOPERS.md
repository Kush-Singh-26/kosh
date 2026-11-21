# Kosh Developer's Guide

This document is for developers who want to contribute to the Kosh core or understand its internal architecture.

## Repository Layout

- `cmd/kosh/`: CLI entry point.
- `cmd/search/`: Go source for the Client-side Search WASM.
- `builder/`: The core SSG engine.
  - `orchestration/`: Manages the build pipeline (clean vs incremental).
  - `parser/`: Unified Markdown parsing and transformations.
  - `generators/`: Site-wide generation (RSS, Sitemap, Graph, etc.).
  - `services/`: Core logic for assets, content, and rendering.
  - `models/`: Strictly typed data structures used across the app.
  - `cache/`: BoltDB-backed persistence for build speed.

## Search WASM Rebuild Process

Kosh uses a client-side search engine written in Go and compiled to WebAssembly.

### When to Rebuild
You must rebuild the search WASM if you:
1. Change any logic in `cmd/search/` or `builder/search/`.
2. Change the search index schema in `builder/models/`.

### Rebuild Command
Kosh includes a helper script to automate the cross-compilation and embedding process:

```bash
go run scripts/rebuild_search.go
```

This script:
- Sets `GOOS=js` and `GOARCH=wasm`.
- Compiles `cmd/search` to `search.wasm`.
- Compresses it and embeds it into `builder/assets/wasm.go`.

## Release Preparation

Before building a release, regenerate the compiled embedded assets:

```bash
# Compile the embedded KaTeX JS into QuickJS bytecode.
go generate ./builder/renderer/native

# Compile/compress the browser search runtime and refresh its source/binary hashes.
go run scripts/rebuild_search.go
```

Keep `builder/assets/wasm/wasm_exec.js` byte-for-byte aligned with the Go toolchain's
`lib/wasm/wasm_exec.js`. The other embedded CSS, JavaScript, templates, and fonts
are source assets and are embedded directly by `go build`.

MessagePack generation is separate: run it when serialized models change, not
simply for a release. Preserve the WASM-excluding build tags on
`builder/models/searchpkg/search_gen.go` and `msgp_methods.go` if regenerating.

Then validate before committing and tagging:

```bash
go mod tidy -diff
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
golangci-lint run ./...
go build -o kosh ./cmd/kosh
```

If the Go build cache produces package-resolution errors under `-race`, retry
with a fresh project-local `GOCACHE` before attributing the failure to the code.

## Cache & Schema Versioning

Kosh uses BoltDB to cache parsed Markdown, rendered fragments, and SSR results (Math/D2).

### Schema Version Alignment
The cache schema (`core.SchemaVersion`) and search schema (`models.CurrentSchemaVersion`) must always be in sync. 

### Migrations
If you change a serialized data structure:
1. Increment the version in `builder/models/models.go` and `builder/cache/core/types.go`.
2. Add a migration step in `builder/cache/migrate/migrations.go` to handle stale caches.

## Rule of Immutability
**Kosh must never mutate the source tree.**
- All build outputs go through `builder/utils/transaction.go` or `builder/utils/sink.go`.
- Avoid hardlinking from source to output to prevent accidental mutation.

## Testing & Benchmarking
Kosh has a robust test suite covering all critical subsystems.

```bash
# Run all tests
go test ./...

# Run targeted speed benchmarks (if site repo is available)
go test -bench . ./builder/orchestration
```

For performance tuning, refer to `docs/perf-baseline.md`.
