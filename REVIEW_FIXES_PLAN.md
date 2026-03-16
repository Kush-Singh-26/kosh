# Review Fixes Plan — 24 Issues

**Generated:** 2026-03-16  
**Target:** Improve strict score from 81.8% to 95.0%  
**Total Issues:** 24 across 5 dimensions

---

## Executive Summary

| Dimension | Issues | Current Score | Target Score | Est. Effort |
|-----------|--------|---------------|--------------|-------------|
| cross_module_architecture | 5 | 68.5% | 85%+ | 4 hours |
| test_strategy | 6 | 68.5% | 85%+ | 6 hours |
| api_surface_coherence | 6 | 72.5% | 88%+ | 3 hours |
| design_coherence | 7 | 72.5% | 88%+ | 4 hours |
| ai_generated_debt | 8 | 71.0% | 85%+ | 2 hours |
| **Total** | **32*** | **~70%** | **90%+** | **~19 hours** |

*Note: Some issues span multiple dimensions

---

## Phase 1: Quick Wins (AI Generated Debt) — 2 hours

### Issue 1: Remove restating comments
**Files:** `builder/search/analyzer.go:10`, `builder/utils/worker_pool.go:12`  
**Effort:** 15 min  
**Action:**
```go
// BEFORE
// English stop words - common words that don't contribute to search relevance
var stopWords = map[string]bool{...}

// AFTER
var stopWords = map[string]bool{...}
```

### Issue 2: Remove pass-through wrapper
**File:** `builder/utils/fs_copy.go:175`  
**Effort:** 10 min  
**Action:** Delete `srcInfoToFileInfo` function, pass `info` directly to `processImageVFS`

### Issue 3: Replace custom HTML escaping with standard library
**File:** `builder/search/engine.go:350`  
**Effort:** 20 min  
**Action:**
```go
// BEFORE
func escapeToBuilder(sb *strings.Builder, s string) { ... }

// AFTER
import "html"
html.EscapeString(s)
```

### Issue 4: Rename generic function
**File:** `builder/utils/assets.go:260`  
**Effort:** 10 min  
**Action:** Rename `normalizeHashCase` → `normalizeEsbuildHashCase`, add comment about Windows case-insensitivity

### Issue 5: Remove nosy comments
**File:** `builder/renderer/native/math.go:10`  
**Effort:** 10 min  
**Action:** Replace WHAT comment with WHY comment explaining Go-to-JS bridge overhead rationale

### Issue 6: Remove defensive overengineering
**File:** `builder/services/post_single.go:19`  
**Effort:** 20 min  
**Action:** Remove panic recovery wrapper unless documented justification exists

### Issue 7: Consolidate buffer pools
**File:** `builder/utils/fs_copy.go:15`  
**Effort:** 30 min  
**Action:** Remove unproven pools, keep only those with benchmark evidence

### Issue 8: Fix restating comments on constants
**File:** `builder/utils/worker_pool.go:12`  
**Effort:** 15 min  
**Action:** Replace with WHY comments (why 32 workers, why buffer multiplier of 4)

---

## Phase 2: API Surface Coherence — 3 hours

### Issue 9: Fix excessive parameters in ParseMarkdown
**File:** `builder/services/post_service.go:156`  
**Effort:** 45 min  
**Action:**
```go
// BEFORE
func ParseMarkdown(ctx, source, path, version, cleanHtmlRelPath, htmlRelPath, mdPool, cfg, nativeRenderer, diagramAdapter, mu, frontmatterHash, readingTime, bodyOffset, preParsedMeta)

// AFTER
type ParseMarkdownConfig struct {
    Ctx context.Context
    Source []byte
    Path string
    Version string
    // ... other fields
}
func ParseMarkdown(cfg ParseMarkdownConfig) (*ParsedMarkdownResult, error)
```

### Issue 10: Standardize copy function parameters
**File:** `builder/utils/fs_copy.go:230`  
**Effort:** 30 min  
**Action:**
```go
// Standardize all to: (ctx, srcFs, sink, srcPath, destPath, options)
type CopyOptions struct {
    ModTime time.Time
    OnWrite func(path string)
    ProcessImage bool
}
```

### Issue 11: Standardize math error handling
**File:** `builder/renderer/native/math.go:167`  
**Effort:** 30 min  
**Action:** Choose fail-fast or collect-all-errors pattern, document it, apply consistently

### Issue 12: Add error contract documentation
**File:** `builder/cache/cache_writes.go:23`  
**Effort:** 20 min  
**Action:**
```go
// BatchCommit atomically commits posts, search records, and dependencies.
// Returns:
//   - error: BoltDB transaction failure (partial commit not possible)
//   - Retries: Safe to retry on failure, idempotent within same build
```

### Issue 13: Fix mixed initialization patterns
**File:** `builder/services/asset_service.go:45`  
**Effort:** 30 min  
**Action:**
```go
// BEFORE
NewAssetService(sourceFs, sink, cfg, renderer, logger)
svc.SetMetrics(m)
svc.SetAssetsReady(ch)

// AFTER
func NewAssetService(sourceFs, sink, cfg, renderer, logger, opts ...AssetServiceOption)
// WithMetrics(m), WithAssetsReady(ch), WithContentAssets(ch)
```

### Issue 14: Split RenderService interface
**File:** `builder/services/interfaces.go:56`  
**Effort:** 45 min  
**Action:**
```go
// BEFORE
type RenderService interface {
    // 13 methods mixing rendering, assets, file tracking
}

// AFTER
type TemplateRenderer interface {
    RenderPage, RenderIndex, Render404, RenderGraph, RenderSidebar, ReloadTemplates
}
type AssetProvider interface {
    SetAssets, GetAssets
}
type FileTracker interface {
    RegisterFile, GetRenderedFiles, ClearRenderedFiles
}
```

---

## Phase 3: Cross-Module Architecture — 4 hours

### Issue 15: Remove cross-layer coupling (WASM deployment)
**File:** `builder/run/builder.go:278`  
**Effort:** 45 min  
**Action:**
```go
// Create builder/services/wasm_service.go
type WasmService interface {
    DeployWASM(ctx context.Context) error
    CheckAndUpdate(ctx context.Context) error
}

// Inject into BuilderDependencies
```

### Issue 16: Split AssetService responsibilities
**File:** `builder/services/asset_service.go:85`  
**Effort:** 60 min  
**Action:**
```go
// Split into:
type AssetDiscovery interface {
    DiscoverAssets(ctx context.Context) ([]ScannedAsset, error)
}
type ImageProcessor interface {
    ProcessImages(ctx context.Context, assets []ScannedAsset) error
}
type BundleBuilder interface {
    BuildBundles(ctx context.Context) (map[string]string, error)
}
```

### Issue 17: Remove GlobalScheduler singleton
**File:** `builder/utils/scheduler.go:47`  
**Effort:** 45 min  
**Action:**
```go
// BEFORE
var GlobalScheduler = NewBuildScheduler()

// AFTER - inject via dependencies
type BuilderDependencies struct {
    Scheduler *BuildScheduler
    // ...
}
```

### Issue 18: Split Cache Manager BatchCommit
**File:** `builder/cache/cache_writes.go:23`  
**Effort:** 60 min  
**Action:**
```go
// Extract domain-specific commit methods
type PostCommitter interface {
    CommitPosts(posts []*PostMeta) error
}
type SearchCommitter interface {
    CommitSearch(records map[string]*SearchRecord) error
}
type DependencyCommitter interface {
    CommitDependencies(deps map[string]*Dependencies) error
}
```

### Issue 19: Reduce builder/run package responsibilities
**File:** `builder/run/build.go:1`  
**Effort:** 60 min  
**Action:**
```go
// Extract services:
type WasmDeployer interface { ... }
type SocialCardManager interface { ... }
type SiteWideRenderer interface { ... }

// Builder coordinates these services instead of implementing logic
```

---

## Phase 4: Design Coherence — 4 hours

### Issue 20: Split Renderer struct
**File:** `builder/renderer/renderer.go:21`  
**Effort:** 60 min  
**Action:**
```go
// BEFORE: 22 fields, 9 function clusters
type Renderer struct {
    Layout, Index, Graph, NotFound, Sidebar *template.Template
    Assets map[string]string
    RenderedSet map[string]bool
    // ... 15 more fields
}

// AFTER
type TemplateRenderer struct {
    Layout, Index, Graph, NotFound, Sidebar *template.Template
    templateDir string
    logger *slog.Logger
}
type AssetManager struct {
    Assets map[string]string
    AssetsMu sync.RWMutex
}
type FileTracker struct {
    RenderedSet map[string]bool
    RenderedMu sync.RWMutex
}
```

### Issue 21: Extract parallel encoding pattern
**File:** `builder/cache/cache_writes.go:23`  
**Effort:** 30 min  
**Action:**
```go
// Extract reusable helper
func ParallelEncode[T any](ctx context.Context, items []T, encoder func(T) ([]byte, error)) ([][]byte, error) {
    var mu sync.Mutex
    results := make([][]byte, len(items))
    var eg errgroup.Group
    for i, item := range items {
        eg.Go(func() error {
            encoded, err := encoder(item)
            if err != nil {
                return err
            }
            mu.Lock()
            results[i] = encoded
            mu.Unlock()
            return nil
        })
    }
    return results, eg.Wait()
}
```

### Issue 22: Extract worker pool invocation pattern
**Files:** `builder/renderer/native/d2.go:27`, `builder/renderer/native/math.go:98`  
**Effort:** 30 min  
**Action:**
```go
// Extract common pattern
func (r *Renderer) invokeWithScheduler(ctx context.Context, task func() (string, error)) (string, error) {
    // acquire/check/defer/release pattern
}
```

### Issue 23: Extract aggregation logic
**Files:** `builder/services/post_service.go:215`, `builder/services/post_single.go:102`  
**Effort:** 30 min  
**Action:**
```go
// Extract shared helper
func (s *postServiceImpl) aggregatePostResult(ctx *postProcessContext, post PostMetadata, searchRecord SearchRecord, renderTask renderTask) {
    // Shared aggregation logic used by both Process and ProcessSingle
}
```

### Issue 24: Group render phase parameters
**File:** `builder/services/post_service.go:125`  
**Effort:** 20 min  
**Action:**
```go
// BEFORE
func (s *postServiceImpl) runRenderPhase(ctx context.Context, numWorkers int, pc *postProcessContext)

// AFTER
type RenderPhaseConfig struct {
    NumWorkers int
    PostsByVersion map[string][]PostMetadata
    PostPosByVersion map[string]map[string]int
}
func (s *postServiceImpl) runRenderPhase(ctx context.Context, cfg RenderPhaseConfig)
```

---

## Phase 5: Test Strategy — 6 hours

### Issue 25: Add integration tests for build pipeline
**File:** `builder/run/build_test.go`  
**Effort:** 90 min  
**Action:**
```go
func TestFullBuildPipeline_Integration(t *testing.T) {
    // Verify: full build completes, incremental rebuilds work,
    // cache is utilized, transaction commit/rollback works
}
```

### Issue 26: Add search engine critical path tests
**File:** `builder/search/engine_test.go`  
**Effort:** 60 min  
**Action:**
```go
func TestSearch_UnicodeHandling(t *testing.T) { ... }
func TestSearch_FuzzyMatchingThresholds(t *testing.T) { ... }
func TestSearch_SnippetExtractionBoundaries(t *testing.T) { ... }
func TestSearch_VersionFilterBehavior(t *testing.T) { ... }
```

### Issue 27: Add parser integration tests
**File:** `builder/parser/parser_test.go` (new)  
**Effort:** 60 min  
**Action:**
```go
func TestParser_FullMarkdownPipeline(t *testing.T) {
    // Verify: markdown -> HTML transformation,
    // math expression collection, D2 diagram SSR caching
}
```

### Issue 28: Add transaction failure tests
**File:** `builder/utils/tx/transaction_test.go`  
**Effort:** 45 min  
**Action:**
```go
func TestTransaction_RenameFailure(t *testing.T) { ... }
func TestTransaction_RollbackRestoresState(t *testing.T) { ... }
func TestTransaction_ConcurrentBuildAttempts(t *testing.T) { ... }
```

### Issue 29: Add concurrent dirty tracking tests
**File:** `builder/services/cache_service_test.go`  
**Effort:** 45 min  
**Action:**
```go
func TestCacheService_ConcurrentDirtyTracking(t *testing.T) {
    // Stress test MarkDirty/ClearDirty under concurrent access
}
```

### Issue 30: Add worker pool concurrency tests
**File:** `builder/utils/worker_pool_test.go` (new)  
**Effort:** 60 min  
**Action:**
```go
func TestWorkerPool_PanicRecovery(t *testing.T) { ... }
func TestWorkerPool_SchedulerTokenReleaseOnCancel(t *testing.T) { ... }
func TestWorkerPool_StopWaitsForInFlightTasks(t *testing.T) { ... }
func TestWorkerPool_SubmitAfterStop(t *testing.T) { ... }
```

---

## Execution Order

### Week 1: Quick Wins + API Coherence (5 hours)
1. ✅ Phase 1: AI Generated Debt (2 hours) - 8 issues
2. ✅ Phase 2: API Surface Coherence (3 hours) - 6 issues

### Week 2: Architecture + Design (8 hours)
3. ✅ Phase 3: Cross-Module Architecture (4 hours) - 5 issues
4. ✅ Phase 4: Design Coherence (4 hours) - 7 issues

### Week 3: Test Coverage (6 hours)
5. ✅ Phase 5: Test Strategy (6 hours) - 6 issues

---

## Score Impact Projections

| Phase | Issues Fixed | Est. Score Gain | Cumulative Score |
|-------|--------------|-----------------|------------------|
| Start | - | - | 81.8% |
| Phase 1 | 8 | +3-5% | 85-87% |
| Phase 2 | 6 | +4-6% | 89-93% |
| Phase 3 | 5 | +5-7% | 94-100% |
| Phase 4 | 7 | +3-5% | 97-100% |
| Phase 5 | 6 | +2-4% | 99-100% |

**Target:** 95.0% achievable after Phase 3-4

---

## Verification Steps

After each phase:
```bash
# Build verification
go build ./...

# Test verification
go test ./builder/... ./cmd/kosh/... -count=1 -timeout 180s

# Desloppify verification
desloppify plan resolve <issue-id> --attest "I have actually fixed..."
desloppify scan
```

After all phases:
```bash
desloppify scan
# Expected: strict score >= 95.0%
```

---

## Risk Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking changes | Low | High | Run full test suite after each phase |
| Test failures | Medium | Low | Fix tests immediately, don't batch |
| Score doesn't improve | Low | Medium | Re-run subjective reviews after fixes |
| Scope creep | Medium | Medium | Stick to 24 identified issues only |

---

## Notes

- **Do not** fix code before importing issues to desloppify (breaks correlation)
- **Do** run `desloppify plan resolve <id>` after each fix with proper attestation
- **Do** commit after each phase for clean history
- **Priority:** Fix issues in order (quick wins first for momentum)

---

## Checklist

- [ ] Phase 1: AI Generated Debt (8 issues) — 2 hours
- [ ] Phase 2: API Surface Coherence (6 issues) — 3 hours
- [ ] Phase 3: Cross-Module Architecture (5 issues) — 4 hours
- [ ] Phase 4: Design Coherence (7 issues) — 4 hours
- [ ] Phase 5: Test Strategy (6 issues) — 6 hours
- [ ] Final scan and score verification

**Total Estimated Effort:** 19 hours  
**Expected Score:** 95-100%
