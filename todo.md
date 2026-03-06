# Kosh vNext Performance Architecture Plan (Cold Build Throughput)

## Working Instructions
- Stay with the original phased plan in this file. Do not replace it with a different architecture track unless a new benchmark proves the original ordering is wrong.
- Phase 0 is already implemented enough to provide real baselines. Do not redo Phase 0 instrumentation unless fixing a concrete bug in the harness or timings.
- Benchmark using the installed `kosh` binary from the real site repo, not `go run` from this engine repo.
- Canonical workflow:
  - from `C:\Users\KIIT0001\blogs`: `go install ./cmd/kosh`
  - from `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`: `kosh clean --cache` then `kosh build --phase-timings`
- Current measured baseline on `C:\Users\KIIT0001\Kush-Singh-26.github.io\blogs-src`:
  - warm-cache cold-output average: `1334ms`
  - clean-cache cold build average: `22916ms`
- Current top clean-build hotspots:
  - asset building: about `3.6s-4.2s`
  - parse: about `4.4s-5.9s`
  - sync-to-disk: about `0.7s-3.1s`
- Render itself is not the dominant bottleneck. Prioritize publish/sync architecture before page-render micro-optimizations.
- Before each phase:
  - record a fresh benchmark run
  - make the smallest change set that proves the phase
  - rerun warm and clean baselines
  - update `docs/perf-baseline.md`
- After each phase:
  - verify output parity against the previous build
  - keep rollback/crash-safety intact
  - avoid mixing unrelated refactors into the same phase
- Immediate next implementation target: Phase 1 only.
- Phase 1 scope discipline:
  - replace `MemMapFs + SyncVFS` with staging-directory streaming publish
  - preserve atomic publish semantics
  - do not mix in DB migration, search schema migration, or scheduler redesign yet
- Root helper files `prev_builder.go` and `test_decode.go` are `//go:build ignore` files. They are not part of production builds. Treat them as archival/debug helpers, not runtime code.

## Objective
- Primary KPI: reduce full cold `kosh build` wall-clock time by **40-60%** on large sites.
- Secondary KPI: keep output correctness, cache integrity, and publish safety unchanged or improved.
- Non-goal (for this phase): watch-mode latency tuning unless it naturally improves.

## Success Criteria
- Throughput:
  - `1k` post benchmark: at least **40% faster** than current baseline.
  - `10k` post benchmark: at least **50% faster** than current baseline target.
- Correctness:
  - HTML output parity (normalized diff) for representative fixtures.
  - Search ranking/snippet regression tests pass.
  - No broken links or asset path regressions.
- Reliability:
  - Build failures never corrupt `outputDir`.
  - `go test ./...` and `go test -race ./...` pass on modified packages.

---

## Phase 0: Baseline + Safety Gates (Mandatory before refactor)

### Deliverables
- Reproducible benchmark harness for cold builds.
- Phase timing instrumentation for current pipeline.
- Golden output fixtures and normalization utilities.
- Hard acceptance gate document with pass/fail thresholds.

### Tasks
1. Add benchmark datasets:
   - small: `100` posts
   - medium: `1k` posts
   - large: `10k` posts
   - variants: image-heavy, math-heavy, mixed.
2. Instrument current phases:
   - assets
   - parse
   - render
   - metadata generators
   - search index
   - disk publish/sync
3. Create benchmark command set:
   - cold output (fresh output dir)
   - warm cache but clean output
4. Add output parity checks:
   - normalized HTML diff
   - sitemap/rss/search artifact integrity checks
5. Record baseline numbers in `docs/perf-baseline.md`.

### Acceptance
- Baseline numbers captured and committed.
- CI and local scripts reproduce within reasonable variance.
- Golden checks fail on real regressions.

---

## Phase 1: Streaming Output + Atomic Directory Publish (Highest ROI) [COMPLETED]

### Goal
Eliminate end-of-build I/O wall from `MemMapFs -> SyncVFS` and distribute writes during build.

### New Architecture
- Add `ArtifactSink` for immediate writes to staging.
- Add `BuildTransaction` to manage `output.tmp` and atomic publish.
- Replace full-tree sync with direct stage writes.

### Interface Additions
```go
type ArtifactSink interface {
    WriteFile(path string, data []byte) error
    WriteStream(path string, fn func(io.Writer) error) error
    MkdirAll(path string) error
    Register(path string)
}

type BuildTransaction interface {
    StagingDir() string
    Commit() error
    Rollback() error
}
```

### Tasks
1. Create staging transaction manager:
   - `outputDir.tmp` lifecycle.
   - atomic swap algorithm:
     - rename `outputDir` -> `outputDir.bak`
     - rename `outputDir.tmp` -> `outputDir`
     - remove `outputDir.bak` on success
   - rollback path on partial failure.
2. Refactor renderer write path:
   - direct write to staging using pooled `bufio.Writer`.
   - remove dependence on `DestFs` for HTML pages.
3. Refactor generators/assets to target sink path directly.
4. Keep rendered-file registry for orphan logic if needed (or remove if made obsolete by full swap).
5. Decommission `SyncVFS` from main cold build path.

### Windows-specific behavior
- Preserve directory-level atomic publish semantics.
- Handle rename edge cases with explicit retry/backoff.
- Never fallback to non-atomic final publish for production mode.

### Acceptance
- Sync phase removed from hot path.
- Build failures leave last-known-good output intact.
- No output corruption across interrupted builds.

### Expected Gain
- Total cold build: **20-35%** improvement on medium/large sites.

---

## Phase 2: Math/D2 In-Flight Deduplication (Low risk, quick win)

### Goal
Ensure each unique SSR fragment (math or D2 hash) renders once per build under concurrency.

### Tasks
1. Introduce keyed in-flight dedupe:
   - `singleflight.Group` or hash->waiter map.
2. Apply to math render path in native renderer.
3. Align D2 path to same primitive (shared helper).
4. Cache successful results and fan out to waiters.

### Acceptance
- Concurrent duplicate renders collapse to single execution.
- No deadlocks under high parallelism.
- Race tests pass.

### Expected Gain
- Math-heavy sites: **5-15%** cold build improvement.
- Typical sites: small but measurable.

---

## Phase 3: Remove Residual Regex HTML Post-Pass (After parity confirmation)

### Goal
Make AST transformations authoritative and remove full-string `ProcessHTML` scanning from render hot path.

### Tasks
1. Catalog every behavior in current HTML post-pass:
   - image extension rewrite
   - path/baseURL/relative prefix handling
   - local path normalization
2. Extend `urlTransformer` + `webpTransformer` to fully cover cases.
3. Introduce temporary feature flag:
   - `EnableLegacyProcessHTML` default true during migration.
4. Run parity suite with flag off; fix deltas.
5. Remove `ProcessHTML` from all render entry points after parity passes.

### Acceptance
- Flag-off output parity achieved on fixtures.
- No URL/image regressions in docs/blog themes.

### Expected Gain
- **5-12%** on render-heavy workloads.

---

## Phase 4: Metadata-First Scheduler (Correctly scoped pipelining)

### Goal
Unblock render scheduling earlier without assuming full global state is available instantly.

### Design
- New `MetadataScanner` pass reads robust frontmatter delimiters (no fixed byte cap).
- Build global indices early:
  - tag map seed
  - version buckets
  - neighbor lookup index
- Separate render classes:
  - class A: pages safe with early metadata
  - class B: aggregate-dependent pages (pagination/tag index/graph) after scanner completion.

### Tasks
1. Implement metadata scanner with strict frontmatter parser.
2. Build immutable metadata snapshot structure.
3. Refactor pipeline into staged DAG with bounded channels:
   - discover -> scan -> parse/analyze -> render(post) -> render(aggregate) -> publish.
4. Ensure ordering guarantees for pages requiring sorted global post sets.

### Acceptance
- First post render starts much earlier in build timeline.
- Aggregate outputs remain deterministic.
- No neighbor/tag/pagination regressions.

### Expected Gain
- **10-20%** on larger sites due to overlap and reduced barrier waiting.

---

## Phase 5: Asset Fast-Path via Hardlink/Reflink + Manifest

### Goal
Avoid redundant copy/write for unchanged, untransformed static assets.

### Tasks
1. Add asset manifest key:
   - source path
   - size
   - mtime
   - optional content hash when ambiguity exists.
2. For unchanged assets:
   - attempt hardlink/reflink into staging.
   - fallback to copy when unsupported/cross-volume.
3. Keep transformed assets on normal processing path (minify/libvips).
4. Validate on Windows NTFS and Linux ext4/xfs.

### Acceptance
- Repeated clean builds skip most raw static copies.
- Identical outputs across link/copy paths.

### Expected Gain
- Asset-heavy sites: **10-25%** phase improvement.

---

## Phase 6: Search Index Build Throughput v2

### Goal
Reduce search generation CPU+memory cost without immediately forcing a consumer format migration.

### Tasks
1. Implement segmented index build:
   - worker-local posting segments
   - merge step
2. Stream encoding to writer instead of giant all-at-once structure where possible.
3. Preserve existing search schema initially (compat-first).
4. Add optional future flag for sharded schema v7 once baseline v2 lands.

### Acceptance
- Search generation time reduced on medium/large datasets.
- WASM client remains compatible for default path.

### Expected Gain
- **5-15%** total on content-heavy sites.

---

## Optional Phase 7: Metadata Store Backend Migration (Only if still bottlenecked)

### Trigger Condition
- Profiling shows cache DB transactions are still a top-3 bottleneck after Phases 1-6.

### Candidate
- Migrate metadata KV from BoltDB to Pebble (LSM) behind `MetaStore` interface.

### Tasks
1. Add backend abstraction.
2. Implement Pebble adapter + migration utility.
3. Keep blob/CAS store unchanged.
4. Validate durability and crash recovery semantics.

### Acceptance
- Demonstrable write-throughput gain under cold build churn.
- No data integrity regressions.

---

## Cross-Cutting Engineering Requirements

### Testing
- Unit tests for all new interfaces and failure paths.
- Integration tests for publish rollback and interrupted builds.
- Golden tests for output parity.
- Benchmark tests pinned to datasets and hardware metadata.

### Observability
- Emit structured per-phase timings and counters.
- Add summary table at end of build:
  - total duration
  - top 3 slowest phases
  - files written/linked/skipped

### Rollout Strategy
1. Land behind feature flags:
   - `KOSH_STREAMING_PUBLISH=1`
   - `KOSH_AST_ONLY_HTML=1`
   - `KOSH_METADATA_PIPELINE=1`
2. Enable in CI benchmark lane first.
3. Enable by default after two consecutive clean benchmark passes and parity stability.

---

## Work Breakdown and Order
1. Phase 0 Baseline + gates
2. Phase 1 Streaming staging publish
3. Phase 2 SSR in-flight dedupe
4. Phase 3 Remove regex post-pass
5. Phase 4 Metadata-first scheduler
6. Phase 5 Asset hardlink/reflink fast-path
7. Phase 6 Search build throughput v2
8. Optional Phase 7 DB backend migration

---

## Final Target
- Cold build throughput improvement: **40-60%** on large real-world sites with no correctness regression and strict atomic publish safety.
