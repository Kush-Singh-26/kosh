# Quality Cleanup Plan — 2026-03-22

## Context
Continuation of code quality improvement work. All 10 original plan phases (Phase 1-10) are COMPLETE.
This plan covers the remaining quality gaps identified after a full codebase scan.

## Goal
Fix remaining type safety issues, duplicate code, dead Engine methods, and test singleton coupling.

---

## Phase A: Type Assertion Safety (~8 sync.Map fixes)

Only fix **sync.Map Range and user-controllable data** assertions. Pool-based (20+) and AST-based (7) assertions are safe by construction — leave as-is.

### Files to fix:
- `builder/assets/image_cache.go:99` — `result[key.(string)] = value.(string)` → safe check
- `builder/fs/sink.go:71` — `cached.(string)` → safe check
- `builder/fs/sink.go:136` — `cached.(string)` → safe check
- `builder/fs/sink.go:276` — sync.Map Range assertion
- `builder/fs/mem_sink.go:41` — sync.Map assertion
- `builder/fs/mem_sink.go:62` — `data.([]byte)` → safe check
- `builder/pools/pools.go:84` — `bw.(*bufio.Writer)` → safe check
- `builder/pools/pools.go:113` — `br.(*bufio.Reader)` → safe check

### Skip (safe by construction):
- All pool `.Get().(*Type)` calls (20 instances)
- All parser AST walking (7 instances) — guarded by `Kind()` checks
- Mock code (4 instances)
- WASM entry (1 instance)
- d2.go ruler pool (1 instance)
- trans_ssr.go goldmark nodes (2 instances)

---

## Phase B: Duplicate Code Cleanup (4 functions)

### B1. `PostChangeType` type + constants (2 locations)
- **Keep:** `builder/orchestration/incremental/manager.go` (canonical)
- **Delete from:** `builder/orchestration/incremental.go` lines 125-132

### B2. `indexedPostStableKey` + `dedupeIndexedPosts` (2 locations)
- **Keep:** `builder/orchestration/search/manager.go` (canonical)
- **Delete from:** `builder/orchestration/incremental.go` lines 16-38
- These are only called by dead `BuildChanged` method (see Phase C)

### B3. `fileExists` (2 locations)
- **Keep:** `builder/cache/store/store.go` (canonical)
- **Delete from:** `builder/orchestration/pipeline_pwa.go` lines 18-21
- Inline `_, err := os.Stat(path); return err == nil` at call site

---

## Phase C: Engine Dead Method Removal (9 methods)

Delete 9 thin delegation methods with zero external callers:

| Method | File | Delegates To |
|--------|------|-------------|
| `normalizeWatchPath` | incremental.go:41 | `b.Watch.NormalizeWatchPath` |
| `isContentPath` | incremental.go:49 | `b.Watch.IsContentPath` |
| `isAssetPath` | incremental.go:58 | `b.Watch.IsAssetPath` |
| `invalidateForTemplate` | incremental.go:68 | `b.Watch.InvalidateForTemplate` |
| `BuildChanged` | incremental.go:75 | `b.Watch.EnqueueChange` |
| `resolveContentPaths` | incremental.go:89 | `b.Incremental.ResolveContentPaths` |
| `computePostHashes` | incremental.go:96 | `b.Incremental.ComputePostHashes` |
| `determinePostChange` | incremental.go:103 | `b.Incremental.DeterminePostChange` |
| `buildSinglePost` | incremental.go:119 | `b.Incremental.BuildSinglePost` |

Update test callers in `incremental_integration_test.go` to use subsystem methods directly.

---

## Phase D: Test Singleton Cleanup (57 calls)

Replace `scheduler.GetGlobalScheduler()` → `scheduler.NewBuildScheduler()` in 9 test files:

1. `builder/orchestration/integration_test.go` (23 calls)
2. `builder/orchestration/incremental_test.go` (6 calls)
3. `builder/orchestration/incremental_integration_test.go` (15 calls)
4. `builder/services/cache/cache_service_test.go` (2 calls)
5. `builder/orchestration/chaos_test.go` (2 calls)
6. `builder/orchestration/build_test.go` (4 calls)
7. `builder/benchmarks/performance_test.go` (2 calls)
8. `builder/services/post/post_service_test.go` (1 call)
9. `builder/services/render/render_service_test.go` (2 calls)

Then remove `GetGlobalScheduler()` from `builder/scheduler/scheduler.go`.

---

## Phase E: Update plan.md

Reflect this session's work (Phase 1-5 cleanup) and new sub-phases in plan.md.

---

## Verification
Run `go vet ./...` and `go test ./...` after each phase.

## Risk Assessment
- All changes are low-to-medium risk
- Most are deletions, mechanical renames, or simple safety checks
- No behavioral changes — all transformations are purely structural
