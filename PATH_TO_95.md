# Path to 95.0% Strict Score

**Current:** 85.1% | **Target:** 95.0% | **Gap:** +9.9%

**Generated:** 2026-03-16  
**Last Scan:** `desloppify scan --path .`

---

## Executive Summary

| Dimension | Current | Target | Gap | Priority | Est. Effort |
|-----------|---------|--------|-----|----------|-------------|
| **Test strategy** | 65.0% | 90% | +25% | P0 | 8 hours |
| **API coherence** | 70.0% | 90% | +20% | P0 | 6 hours |
| **Cross-module arch** | 72.0% | 90% | +18% | P1 | 8 hours |
| **Stale migration** | 75.0% | 90% | +15% | P1 | 4 hours |
| **Design coherence** | 78.0% | 90% | +12% | P2 | 4 hours |
| **Error consistency** | 78.0% | 90% | +12% | P2 | 3 hours |
| **All others** | 80-96% | 90% | varies | P3 | 4 hours |
| **Total** | **85.1%** | **95.0%** | **+9.9%** | - | **~37 hours** |

---

## Phase 1: Quick Wins (Test Strategy) — 8 hours

**Goal:** Lift test_strategy from 65% → 90% (+25%)

### 1.1 Add Integration Tests for Critical Paths

**Files to create:**
- `builder/run/integration_test.go` (new)
- `builder/services/cache_integration_test.go` (new)

**Tests to add:**

```go
// builder/run/integration_test.go
func TestFullBuildPipeline_Integration(t *testing.T)
func TestIncrementalBuild_CacheUtilization(t *testing.T)
func TestCleanBuild_Reproducibility(t *testing.T)
func TestTransactionFailure_Rollback(t *testing.T)

// builder/services/cache_integration_test.go
func TestCacheService_DirtyTrackingIntegration(t *testing.T)
func TestCacheService_BatchCommitIntegration(t *testing.T)
func TestCacheService_SocialCardHashPersistence(t *testing.T)
```

**Effort:** 3 hours  
**Expected gain:** +8-10%

---

### 1.2 Add Concurrency Stress Tests

**Files to create:**
- `builder/utils/concurrency_test.go` (new)
- `builder/services/concurrent_rebuild_test.go` (new)

**Tests to add:**

```go
// builder/utils/concurrency_test.go
func TestWorkerPool_RaceConditionFree(t *testing.T)
func TestScheduler_TokenExhaustion(t *testing.T)
func TestImageCache_ConcurrentAccess(t *testing.T)

// builder/services/concurrent_rebuild_test.go
func TestPostService_ConcurrentRebuild(t *testing.T)
func TestRenderService_ConcurrentTemplateReload(t *testing.T)
func TestAssetService_ConcurrentBuild(t *testing.T)
```

**Effort:** 3 hours  
**Expected gain:** +8-10%

---

### 1.3 Add Schema Migration Tests

**Files to modify:**
- `builder/cache/migrations_test.go` (extend)

**Tests to add:**

```go
func TestCacheSchema_MigrationPath(t *testing.T)
func TestCacheSchema_VersionMismatchHandling(t *testing.T)
func TestSearchWASM_SchemaCompatibility(t *testing.T)
```

**Effort:** 2 hours  
**Expected gain:** +5-7%

---

## Phase 2: API Coherence — 6 hours

**Goal:** Lift api_surface_coherence from 70% → 90% (+20%)

### 2.1 Standardize Constructor Patterns

**Current state:**
- PostService: dependency struct ✅
- AssetService: variadic options ✅
- RenderService: positional parameters ❌
- CacheService: positional parameters ❌

**Action:** Convert RenderService and CacheService to dependency struct pattern

**Files to modify:**
- `builder/services/interfaces.go`
- `builder/services/render_service.go`
- `builder/services/cache_service.go`
- `builder/run/builder.go`

**Effort:** 3 hours  
**Expected gain:** +8-10%

---

### 2.2 Document Error Handling Strategy

**Action:** Add error contract documentation to all service interfaces

**Files to modify:**
- `builder/services/interfaces.go` — add error contracts to each interface
- `builder/cache/cache_writes.go` — extend BatchCommit docs
- `builder/utils/tx/transaction.go` — document retry behavior

**Example:**

```go
// PostService handles markdown parsing and post processing.
//
// Error Contract:
//   - Parse errors: returned immediately, caller must handle
//   - Cache errors: logged, build continues (best-effort)
//   - Filesystem errors: returned, build halts
//
// Concurrency:
//   - Process: safe for concurrent calls (streaming mode)
//   - ProcessSingle: not thread-safe for same path
type PostService interface {
    // ...
}
```

**Effort:** 2 hours  
**Expected gain:** +5-7%

---

### 2.3 Fix Channel Ownership Patterns

**Issue:** AssetsReady channel ownership unclear across services

**Action:** Document channel lifecycle in AssetService

**Files to modify:**
- `builder/services/asset_service.go`
- `builder/services/post_service.go`
- `builder/services/render_service.go`

**Effort:** 1 hour  
**Expected gain:** +3-5%

---

## Phase 3: Cross-Module Architecture — 8 hours

**Goal:** Lift cross_module_architecture from 72% → 90% (+18%)

### 3.1 Consolidate Cache Interfaces

**Current:** 8 service-specific cache interfaces  
**Target:** 3 focused interfaces

**Action:** Merge overlapping interfaces

```go
// BEFORE
type PostServiceCache interface { PostCache; ContentCache; SocialCardCache; DirtyTracker; ... }
type RenderServiceCache interface { ContentCache }
type AssetServiceCache interface { BuildArtifactCache }
type ScannerCache interface { PostCache }
type SiteGeneratorCache interface { BuildArtifactCache; PostCache }

// AFTER
type MetadataCache interface { PostCache; DirtyTracker }
type ContentStore interface { ContentCache; SocialCardCache }
type ArtifactCache interface { BuildArtifactCache }
```

**Files to modify:**
- `builder/services/interfaces.go`
- `builder/cache/cache_writes.go`
- All service implementations

**Effort:** 3 hours  
**Expected gain:** +6-8%

---

### 3.2 Extract WASM Service

**Issue:** WASM deployment logic in builder/run violates separation of concerns

**Action:** Create dedicated WASM service

**Files to create:**
- `builder/services/wasm_service.go` (new)

**Files to modify:**
- `builder/run/builder.go`
- `builder/run/build.go`
- `builder/assets/wasm.go`

**Interface:**

```go
type WasmService interface {
    // CheckAndUpdate compiles WASM if source changed or missing
    CheckAndUpdate(ctx context.Context) error
    // Deploy copies WASM to staging directory
    Deploy(ctx context.Context) error
    // Hash returns current WASM hash for cache invalidation
    Hash() (string, error)
}
```

**Effort:** 3 hours  
**Expected gain:** +6-8%

---

### 3.3 Remove RenderService Delegate Pattern

**Issue:** RenderService is thin wrapper (14 methods, all delegate)

**Action:** Inline RenderService or convert to facade with clear value-add

**Options:**
1. **Inline:** Remove RenderService, use renderer.Renderer directly
2. **Facade:** Keep RenderService but add caching/template-reload coordination

**Recommendation:** Option 2 (Facade) — preserves abstraction for future template hot-reload

**Files to modify:**
- `builder/services/render_service.go`
- `builder/renderer/renderer.go`

**Effort:** 2 hours  
**Expected gain:** +4-6%

---

## Phase 4: Stale Migration — 4 hours

**Goal:** Lift incomplete_migration from 75% → 90% (+15%)

### 4.1 Remove EnableLegacyProcessHTML Flag

**Current:** `renderer.EnableLegacyProcessHTML` flag indicates coexisting patterns

**Action:** Complete migration to new HTML processing, remove flag

**Files to modify:**
- `builder/renderer/renderer.go`
- `builder/parser/trans_ssr.go`
- `builder/services/post_parser.go`

**Steps:**
1. Verify all posts use new transformer pipeline
2. Remove legacy regex post-pass
3. Remove flag from Config
4. Update tests

**Effort:** 2 hours  
**Expected gain:** +8-10%

---

### 4.2 Consolidate Copy Implementations

**Current:** Multiple copy implementations (optimized vs fallback)

**Action:** Document platform-specific behavior, consolidate where possible

**Files to review:**
- `builder/utils/copy_optimized_*.go`
- `builder/utils/copy_fallback.go`
- `builder/utils/fs_copy.go`

**Effort:** 2 hours  
**Expected gain:** +5-7%

---

## Phase 5: Design Coherence — 4 hours

**Goal:** Lift design_coherence from 78% → 90% (+12%)

### 5.1 Fix ParseOptions Design

**Issue:** ParseOptions mixes config with runtime dependencies

**Action:** Split into ParseConfig + ParseContext

```go
// BEFORE
type ParseOptions struct {
    Source []byte
    Path string
    // ... config fields ...
    Mu *sync.Mutex  // unused!
    NativeRenderer *native.Renderer  // runtime dependency
}

// AFTER
type ParseConfig struct {
    Source []byte
    Path string
    Version string
    // ... pure config only ...
}

type ParseContext struct {
    Mu sync.Mutex
    NativeRenderer *native.Renderer
    DiagramAdapter *cache.DiagramCacheAdapter
}
```

**Files to modify:**
- `builder/services/post_parser.go`
- `builder/services/post_service.go`
- `builder/services/post_single.go`

**Effort:** 2 hours  
**Expected gain:** +6-8%

---

### 5.2 Remove Unused Fields

**Issue:** `Mu *sync.Mutex` in ParseOptions is unused

**Action:** Remove dead code

**Files to modify:**
- `builder/services/post_parser.go`

**Effort:** 30 min  
**Expected gain:** +1-2%

---

### 5.3 Fix Math Batching Hardcoding

**Issue:** Math batch chunk size hardcoded

**Action:** Make configurable via ParseContext

**Files to modify:**
- `builder/renderer/native/math.go`

**Effort:** 1.5 hours  
**Expected gain:** +3-4%

---

## Phase 6: Error Consistency — 3 hours

**Goal:** Lift error_consistency from 78% → 90% (+12%)

### 6.1 Document Fire-and-Forget Patterns

**Issue:** Mixed error strategies (fire-and-forget vs immediate return)

**Action:** Document which operations are best-effort

**Files to modify:**
- `builder/services/post_service.go` — cache commit docs
- `builder/cache/cache_writes.go` — BatchCommit docs
- `builder/run/build.go` — async operation docs

**Effort:** 1.5 hours  
**Expected gain:** +5-7%

---

### 6.2 Fix Silent Error Swallowing

**Issue:** `processCachedMath` ignores diagramAdapter errors

**Action:** Log or return errors consistently

**Files to modify:**
- `builder/services/post_service.go`

**Effort:** 1.5 hours  
**Expected gain:** +5-7%

---

## Phase 7: Elegance Improvements — 4 hours

**Goal:** Lift elegance scores from 85.7% → 92% (+6.3%)

### 7.1 Remove searchSession Pattern

**Issue:** Stateless type with receiver methods

**Action:** Convert to package-level functions

**Files to modify:**
- `builder/search/engine.go`

**Effort:** 1 hour  
**Expected gain:** +2-3%

---

### 7.2 Fix escapeToBuilder Pass-Through

**Issue:** Single-line wrapper around html.EscapeString

**Action:** Inline or remove

**Files to modify:**
- `builder/search/engine.go`

**Effort:** 30 min  
**Expected gain:** +1-2%

---

### 7.3 Fix WorkerPool Error Propagation

**Issue:** Handler `func(T)` provides no error return

**Action:** Add error channel or callback

**Files to modify:**
- `builder/utils/worker_pool.go`

**Effort:** 2.5 hours  
**Expected gain:** +3-4%

---

## Execution Order

### Week 1: Test Strategy (8 hours)
- [ ] 1.1 Integration tests (3h)
- [ ] 1.2 Concurrency tests (3h)
- [ ] 1.3 Schema migration tests (2h)

**Expected score:** 85.1% → **88-89%**

---

### Week 2: API Coherence (6 hours)
- [ ] 2.1 Standardize constructors (3h)
- [ ] 2.2 Document error contracts (2h)
- [ ] 2.3 Fix channel ownership (1h)

**Expected score:** 88% → **90-91%**

---

### Week 3: Cross-Module Architecture (8 hours)
- [ ] 3.1 Consolidate cache interfaces (3h)
- [ ] 3.2 Extract WASM service (3h)
- [ ] 3.3 Fix RenderService pattern (2h)

**Expected score:** 91% → **92-93%**

---

### Week 4: Remaining Gaps (8 hours)
- [ ] Phase 4: Stale migration (4h)
- [ ] Phase 5: Design coherence (4h)
- [ ] Phase 6: Error consistency (3h)
- [ ] Phase 7: Elegance (4h)

**Expected score:** 93% → **95%+**

---

## Verification Steps

After each phase:

```bash
# Run tests
go test ./builder/... ./cmd/kosh/... -count=1 -timeout 180s

# Build verification
go build ./...

# Desloppify scan
python -m desloppify scan --path .

# Check progress
python -m desloppify status
```

---

## Risk Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking changes | Low | High | Run full test suite after each phase |
| Test failures | Medium | Low | Fix tests immediately, don't batch |
| Score doesn't improve | Low | Medium | Re-run subjective reviews after fixes |
| Scope creep | Medium | Medium | Stick to plan, defer new findings |

---

## Success Criteria

- [ ] Strict score ≥ 95.0%
- [ ] All 18 test packages pass
- [ ] No build errors
- [ ] No regression in existing functionality
- [ ] All new tests pass with `-race` flag

---

## Notes

- **Do not** skip test writing — test strategy is the biggest gap
- **Do** run `desloppify next` after each phase to catch new issues
- **Do** commit after each phase for clean history
- **Priority:** Test strategy > API coherence > Architecture > Elegance

---

**Total Estimated Effort:** 37 hours  
**Expected Final Score:** 95-97%
