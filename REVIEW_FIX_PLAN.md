# Review Fix Plan

**Goal:** Fix open subjective review issues to improve strict score from 87.2 → 95.0

**Current State:**
- Strict Score: 87.2/100 (target: 95.0)
- Open Review Issues: 1 (T1-High)

## Completed Fixes ✅

1. **AssetProvider unnecessary indirection** - REMOVED interface, embedded methods in RenderService
2. **FireAndForget error swallowing** - ADDED FireAndForgetWithResult and FireAndForgetWithMetrics

## Remaining Issue

1. **CacheService composite interface** - 25+ methods, 5 unrelated concerns

---

## Issue #1: CacheService Composite Interface [T1-High]

**Problem:** CacheService has 25+ methods spanning 5 unrelated concerns:
- PostCache (post metadata caching)
- ContentCache (HTML content caching)
- SocialCardCache (social card image caching)
- BuildArtifactCache (search/graph/wasm caching)
- DirtyTracker (dirty marking for incremental builds)

**Files:**
- `builder/services/interfaces.go` - Interface definition
- `builder/services/cache_service.go` - Implementation
- `builder/cache/cache.go` - Underlying cache.Manager

**Impact:** Most callers only need a subset of capabilities but must depend on the wide composite interface.

**Fix Strategy:**
1. Keep the focused sub-interfaces as primary injection points
2. Instead of composing them into CacheService, inject only the needed sub-interface into each service
3. Update service constructors to accept specific sub-interfaces

**Services and their actual cache needs:**
- `PostService` → needs `PostCache` + `DirtyTracker`
- `RenderService` → needs `ContentCache`
- `AssetService` → needs `BuildArtifactCache`
- `Scanner` → needs `PostCache` (read-only)

**Steps:**
1. Update `builder/services/interfaces.go` to remove CacheService composite
2. Update each service constructor to accept specific sub-interface
3. Update `builder/run/build.go` to wire specific interfaces
4. Remove `cacheServiceImpl` wrapper if no longer needed

**Expected Impact:** +2-3 points on abstraction_fitness, +1-2 on design_coherence

---

## Issue #2: AssetProvider Unnecessary Indirection [T2-Medium]

**Problem:** AssetProvider interface (SetAssets, GetAssets) adds indirection when:
- Renderer already manages assets with atomic snapshots
- Single implementation only (no testability benefit)
- renderServiceImpl delegates directly to underlying *renderer.Renderer

**Files:**
- `builder/services/interfaces.go` - AssetProvider interface
- `builder/services/render_service.go` - RenderService embeds AssetProvider
- `builder/renderer/renderer.go` - Renderer has atomic asset snapshot

**Fix Strategy:**
1. Remove AssetProvider interface from interfaces.go
2. Embed asset methods directly in RenderService interface
3. Update Renderer to expose GetAssets/SetAssets directly
4. Update tests to use Renderer directly

**Steps:**
1. Delete AssetProvider interface definition
2. Add `GetAssets() map[string]string` and `SetAssets(map[string]string)` to RenderService interface
3. Update renderServiceImpl to call renderer methods directly
4. Update any mock implementations

**Expected Impact:** +1-2 points on abstraction_fitness

---

## Issue #3: FireAndForget Error Swallowing [T2-Medium]

**Problem:** FireAndForget utilities intentionally swallow errors by logging only:
- No mechanism for callers to track failures
- No retry/recovery capability
- Used for cache commits where failures could lead to stale state

**Files:**
- `builder/utils/async/async.go` - FireAndForget utilities
- `builder/services/post_service.go:426` - Cache commit usage
- `builder/run/build.go` - Various background operations

**Fix Strategy:**
1. Add `FireAndForgetWithResult` variant that returns `<-chan error`
2. Add metrics counter integration for monitoring failure rates
3. For critical operations (cache commits), use the result channel variant

**New API:**
```go
// FireAndForgetWithResult runs fn in background and returns error channel
// Caller can select on channel to track completion/failure
func FireAndForgetWithResult[T any](fn func() (T, error)) <-chan error

// FireAndForgetWithMetrics adds metrics tracking for failure rates
func FireAndForgetWithMetrics(fn func() error, metricName string)
```

**Steps:**
1. Add new functions to `builder/utils/async/async.go`
2. Update cache commit in post_service.go to use result channel
3. Add metrics tracking for background operation failures
4. Document when to use each variant

**Expected Impact:** +1-2 points on error_consistency, +1 on design_coherence

---

## Execution Order

1. **Issue #1 (CacheService)** - Highest impact, most invasive
   - Estimated effort: 2-3 hours
   - Risk: Medium (interface changes)
   - Test impact: Update service constructor tests

2. **Issue #2 (AssetProvider)** - Quick win
   - Estimated effort: 30 minutes
   - Risk: Low (single implementation)
   - Test impact: Minimal

3. **Issue #3 (FireAndForget)** - Medium complexity
   - Estimated effort: 1 hour
   - Risk: Low (additive changes)
   - Test impact: Add tests for new variants

---

## Verification

After all fixes:
1. Run `go build ./...` - verify compilation
2. Run `go test ./...` - verify all tests pass
3. Run `python -m desloppify scan` - verify score improvement
4. Run `python -m desloppify show review --status open` - verify issues resolved

**Target Scores After Fix:**
- Strict: 87.7 → 92.0+ (then re-review remaining dimensions for 95.0)

---

## Follow-up Reviews

After fixing these 3 issues, run follow-up subjective reviews for lowest dimensions:
- `convention_outlier` (78.5%) - Address boilerplate duplication
- `initialization_coupling` (82.0%) - Review dependency injection patterns
- `design_coherence` (82.5%) - Review overall design consistency
