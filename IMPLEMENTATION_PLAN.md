# Kosh Implementation Plan - Road to 95.0

**Current Status:** 84.4/100 (target: 95.0, need +10.6)  
**Last Updated:** 2026-03-15  
**Commits:** 21 ahead of origin/main

---

## Executive Summary

### Score Progress
| Dimension | Baseline | Current | Change | Priority |
|-----------|----------|---------|--------|----------|
| **High-level elegance** | 80.0% | 88.5% | +8.5 | ✅ Done |
| **Design coherence** | 72.5% | 82.5% | +10.0 | ✅ Done |
| **Cross-module arch** | 68.5% | 72.5% | +4.0 | 🔄 In Progress |
| **Mid-level elegance** | 78.5% | 78.5% | 0 | ⏳ Pending |
| **Test strategy** | 68.5% | 68.5% | 0 | ⏳ Deferred |
| **AI generated debt** | 71.0% | 71.0% | 0 | ⏳ Deferred |

### Completed Improvements
- ✅ Split Builder into BuilderDependencies + BuilderState
- ✅ Consolidated service interfaces with ReconfigureForBuild()
- ✅ Channel-based synchronization replacing callbacks
- ✅ Extensive documentation for integration seams
- ✅ Fire-and-forget error logging pattern

---

## Phase 1: Architectural Decoupling (High Impact, ~+4.0 points)

### 1.1 Fix Cache→Renderer Upward Dependency [T1-High]
**Issue:** `builder/cache/types.go` imports `builder/renderer/native` for SSR types  
**Impact:** Violates layering - cache should be infrastructure, not depend on renderer  
**Files:** `builder/cache/types.go`, `builder/cache/types_gen.go`, `builder/cache/adapter.go`

**Implementation:**
```go
// Step 1: Move SSR types to builder/models
// Create: builder/models/ssr.go
package models

type SSRArtifactType int
const (
    SSRLaTeX SSRArtifactType = iota
    SSRD2
)

type SSRKey struct {
    Type    SSRArtifactType
    Content string
    Hash    string
}

// Step 2: Update cache to store opaque blobs
// Modify: builder/cache/types.go
package cache

// Remove: import "github.com/Kush-Singh-26/kosh/builder/renderer/native"
// Replace typed methods with generic blob storage
func (c *Cache) StoreSSR(key SSRKey, data []byte) error
func (c *Cache) GetSSR(key SSRKey) ([]byte, error)

// Step 3: Update adapter to use new types
// Modify: builder/cache/adapter.go
package cache

type DiagramCacheAdapter struct {
    cache CacheService
}

func (d *DiagramCacheAdapter) Get(key string) (string, bool) {
    data, err := d.cache.GetSSR(SSRKey{Type: SSRD2, Content: key})
    // ...
}
```

**Estimated Effort:** 2-3 hours  
**Risk:** Medium - requires updating all SSR cache call sites  
**Testing:** Verify D2/LaTeX SSR cache hit/miss behavior unchanged

---

### 1.2 Decouple WASM Deployment from internal/build [T2-Medium]
**Issue:** `builder/run` imports `internal/build` for WASM deployment  
**Impact:** Blurs layer responsibilities - builder should own build-time assets  
**Files:** `builder/run/builder.go`, `internal/build/build.go`

**Implementation:**
```go
// Step 1: Create builder/wasm/deploy.go
package wasm

//go:embed wasm/search.wasm.br
var searchWasm []byte

type Deployer struct {
    outputDir string
    logger    *slog.Logger
}

func (d *Deployer) DeploySearchWasm() error {
    // Move logic from internal/build/build.go
}

// Step 2: Update builder/run/builder.go
// Remove: import "github.com/Kush-Singh-26/kosh/internal/build"
// Add: import "github.com/Kush-Singh-26/kosh/builder/wasm"

func (b *Builder) deployWasm() {
    deployer := wasm.NewDeployer(b.cfg.OutputDir, b.logger)
    deployer.DeploySearchWasm()
}

// Step 3: Update internal/build to use builder/wasm
package build

import "github.com/Kush-Singh-26/kosh/builder/wasm"

func CheckWasmUpdate(...) {
    // Use wasm.Deployer instead of direct logic
}
```

**Estimated Effort:** 1-2 hours  
**Risk:** Low - mostly moving code  
**Testing:** Verify search WASM deploys correctly in build and dev modes

---

### 1.3 Split builder/utils into Subpackages [T1-High]
**Issue:** 48 files, ~4000 LOC, 47 dependents - architectural gravity well  
**Impact:** Low cohesion, high coupling, difficult to test  
**Files:** All `builder/utils/*.go`

**Implementation:**
```
builder/utils/                    → Split into:
├── transaction.go                → builder/tx/
├── sink.go                       → builder/sink/
├── assets.go                     → builder/assets/ (already exists, merge)
├── fs_copy.go                    → builder/io/
├── fs_path.go                    → builder/io/
├── worker_pool.go                → builder/sync/
├── sync.go                       → builder/sync/
├── image_*.go                    → builder/image/
├── hash.go                       → builder/hash/
└── timer.go                      → builder/metrics/ (or keep in utils)
```

**Phase 1.3a: Extract Transaction (Highest Priority)**
```go
// Create: builder/tx/transaction.go
package tx

type BuildTransaction struct {
    // Move from builder/utils/transaction.go
}

type DirectoryTx struct {
    // Move from builder/utils/transaction.go
}

// Create: builder/tx/staging.go
package tx

type StagingDir struct {
    // Extract staging directory logic
}
```

**Phase 1.3b: Extract Sink**
```go
// Create: builder/sink/sink.go
package sink

type ArtifactSink interface {
    // Move interface from builder/utils/sink.go
}

type DiskSink struct {
    // Move implementation
}

type MemorySink struct {
    // Move implementation
}
```

**Phase 1.3c: Extract IO Utilities**
```go
// Create: builder/io/copy.go
package io

func CopyFileVFS(...) error
func CopyDirVFS(...) error
// ... fs_copy.go, fs_path.go functions
```

**Estimated Effort:** 6-8 hours (spread across multiple sessions)  
**Risk:** High - many dependents, requires careful import updates  
**Testing:** Full build + incremental dev mode tests essential

---

## Phase 2: Mid-Level Elegance (~+3.0 points)

### 2.1 Consolidate Dual Sync Channels [T1-High]
**Issue:** assetsReady + metadataReadyCh create redundant synchronization  
**Files:** `builder/run/build.go`, `builder/services/*.go`

**Implementation:**
```go
// Create: builder/run/build_context.go
package run

// BuildSync provides synchronized build phase coordination
type BuildSync struct {
    // AssetsReady closes when asset building completes
    AssetsReady <-chan struct{}
    
    // MetadataReady closes when post metadata is parsed
    MetadataReady <-chan struct{}
    
    // Err collects async errors from all phases
    Err <-chan error
}

// Update: builder/services/interfaces.go
type PostService interface {
    // Remove: MetadataReadyChan() <-chan struct{}
    // Use BuildSync instead
}

type AssetService interface {
    // Remove: SetAssetsReadySignal(ch chan struct{})
    // Use BuildSync instead
}

// Update: builder/run/build.go
func (b *Builder) setupBuildSync() *BuildSync {
    sync := &BuildSync{
        AssetsReady:   make(chan struct{}),
        MetadataReady: make(chan struct{}),
        Err:           make(chan error, 3),
    }
    // Wire up services to use sync
    return sync
}
```

**Estimated Effort:** 2-3 hours  
**Risk:** Medium - affects build orchestration flow  
**Testing:** Verify site-wide generators receive correct signals

---

### 2.2 Standardize Fire-and-Forget Error Logging [T1-High]
**Issue:** Inconsistent error logging across async goroutines  
**Files:** `builder/services/post_service.go`, `builder/run/pipeline_pagination.go`, `builder/utils/sink.go`

**Implementation:**
```go
// Create: builder/sync/async.go
package sync

// FireAndForget wraps a function with standard error logging
func FireAndForget(name string, fn func() error, logger *slog.Logger) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                logger.Error("Panic in fire-and-forget", 
                    "operation", name, 
                    "panic", r)
            }
        }()
        if err := fn(); err != nil {
            logger.Error("Error in fire-and-forget",
                "operation", name,
                "error", err)
        }
    }()
}

// Update: builder/services/post_service.go
func (s *postServiceImpl) finalizeBuild(pc *postProcessContext) {
    if len(pc.newPostsMeta) > 0 && s.cache != nil {
        s.cacheWg.Add(1)
        sync.FireAndForget("cache_commit", func() error {
            return s.cache.BatchCommit(...)
        }, s.logger)
    }
}

// Update: builder/utils/sink.go
func (s *DiskSink) WriteStream(...) error {
    defer func() {
        bw.Reset(nil)
        s.bufPool.Put(bw)
    }()
    
    // Wrap fn call
    defer func() {
        if r := recover(); r != nil {
            if tempPath != "" {
                _ = s.fs.Remove(tempPath)
            }
            panic(r) // Re-throw after cleanup
        }
    }()
    
    // ... rest of logic
}
```

**Estimated Effort:** 1-2 hours  
**Risk:** Low - additive pattern, doesn't change behavior  
**Testing:** Verify error logs appear correctly

---

### 2.3 Deduplicate Cache Commit Logic [T1-High]
**Issue:** BatchCommit and incremental path duplicate encoding logic  
**Files:** `builder/cache/cache_writes.go`, `builder/services/post_single.go`

**Implementation:**
```go
// Create: builder/cache/commit_builder.go
package cache

type CommitBuilder struct {
    posts       []PostMeta
    searchRecs  []SearchRecord
    deps        map[string][]string
}

func NewCommitBuilder() *CommitBuilder { return &CommitBuilder{} }

func (b *CommitBuilder) AddPost(meta PostMeta) *CommitBuilder {
    b.posts = append(b.posts, meta)
    return b
}

func (b *CommitBuilder) AddSearchRecord(rec SearchRecord) *CommitBuilder {
    b.searchRecs = append(b.searchRecs, rec)
    return b
}

func (b *CommitBuilder) AddDep(path, dep string) *CommitBuilder {
    if b.deps == nil {
        b.deps = make(map[string][]string)
    }
    b.deps[path] = append(b.deps[path], dep)
    return b
}

func (b *CommitBuilder) Commit(cache CacheService) error {
    // Single implementation of encoding + commit logic
    // Uses errgroup for parallel encoding
    // Called by both BatchCommit and incremental path
}

// Update: builder/cache/cache_writes.go
func (c *cacheService) BatchCommit(posts, searchRecs, deps) error {
    builder := NewCommitBuilder()
    for _, p := range posts {
        builder.AddPost(p)
    }
    // ...
    return builder.Commit(c)
}

// Update: builder/services/post_single.go
func (s *postServiceImpl) commitSingle(meta, searchRec, deps) error {
    builder := cache.NewCommitBuilder()
    builder.AddPost(meta).AddSearchRecord(searchRec)
    return builder.Commit(s.cache)
}
```

**Estimated Effort:** 1-2 hours  
**Risk:** Low - refactoring, behavior unchanged  
**Testing:** Verify cache commits work in full and incremental builds

---

## Phase 3: Search & Renderer Improvements (~+2.0 points)

### 3.1 Extract Search Scoring Strategies [T1-High]
**Issue:** engine.go has 588 LOC, complexity 71  
**Files:** `builder/search/engine.go`

**Implementation:**
```
builder/search/
├── engine.go          → Keep as coordinator (reduce to ~150 LOC)
├── scoring/           → New directory
│   ├── bm25.go        → BM25 scoring logic
│   ├── phrase.go      → Phrase matching
│   ├── boost.go       → Title/tag boosts
│   └── scorer.go      → Scorer interface
├── indexing/          → New directory
│   ├── ngram.go       → N-gram indexing
│   └── indexer.go     → Indexer interface
└── query/             → New directory
    ├── parse.go       → Query parsing
    └── analyzer.go    → Query analysis
```

**Estimated Effort:** 3-4 hours  
**Risk:** Medium - algorithmic code, needs careful testing  
**Testing:** Verify search results identical before/after

---

### 3.2 Split Renderer Responsibilities [T1-High]
**Issue:** Renderer struct combines template management, asset caching, render execution  
**Files:** `builder/renderer/renderer.go`

**Implementation:**
```go
// Create: builder/renderer/template_manager.go
package renderer

type TemplateManager struct {
    templates *template.Template
    // Template loading, reloading, parsing
}

// Create: builder/renderer/asset_cache.go
package renderer

type AssetCache struct {
    assets map[string]string
    // Asset storage and retrieval
}

// Create: builder/renderer/page_renderer.go
package renderer

type PageRenderer struct {
    tmplMgr *TemplateManager
    assets  *AssetCache
    // RenderPage, RenderIndex, etc.
}

// Update: builder/renderer/renderer.go
type Renderer struct {
    tmplMgr *TemplateManager
    assets  *AssetCache
    pageRnd *PageRenderer
    // Delegate methods
}
```

**Estimated Effort:** 2-3 hours  
**Risk:** Medium - affects all page rendering  
**Testing:** Full site build verification essential

---

## Phase 4: Test Strategy (Deferred, ~+2.0 points)

**Note:** Test strategy improvements are deferred until after architectural fixes.  
Adding tests before refactoring would create resistance to change.

### 4.1 Add Search Package Tests
- BM25 scoring unit tests
- Query parsing tests
- Index build tests

### 4.2 Add Cache Service Tests
- BatchCommit tests
- Incremental commit tests
- SSR cache tests

### 4.3 Add Integration Tests
- Full build → output verification
- Incremental rebuild → correctness
- Dev mode → live reload

---

## Phase 5: AI Generated Debt (Deferred)

**Note:** AI debt is a meta-issue - addressed by ensuring all new code is human-reviewed.

---

## Risk Mitigation

### High-Risk Changes
1. **Utils split** - Do incrementally, one subpackage at a time
2. **Cache decoupling** - Maintain backward compat during transition
3. **Sync consolidation** - Test in dev mode extensively

### Testing Strategy
- After each phase: `go test ./...` + `kosh clean` + `kosh serve --dev`
- Verify: search works, CSS updates apply, incremental rebuilds fast
- Benchmark: warm build time should not regress

---

## Success Criteria

| Phase | Target Score | Key Deliverables |
|-------|--------------|------------------|
| Phase 1 | 88.0 | Cache decoupled, WASM moved, utils split |
| Phase 2 | 91.0 | Sync consolidated, errors standardized, deduped |
| Phase 3 | 93.0 | Search extracted, renderer split |
| Phase 4 | 95.0 | Tests added for critical paths |

---

## Next Actions

1. **Start with 1.1** (Cache→Renderer decoupling) - highest impact, clearest fix
2. **Then 1.2** (WASM decoupling) - quick win, low risk
3. **Then 1.3a** (Extract tx package) - first step of utils split
4. **Pause and rescan** after Phase 1 to reassess priorities

---

## Appendix: Issue Tracker

### T1-High Issues (9)
- [ ] cache_renderer_upward_dep
- [ ] channel_ownership_documented (positive pattern)
- [ ] reconfigure_pattern_consolidated (positive pattern)
- [ ] search_complexity_concentration (acceptable isolation)
- [ ] utils_gravity_well
- [ ] duplication_cache_commit_logic
- [ ] dual_sync_channels
- [ ] fire_and_forget_error_logging
- [ ] sink_stream_panic_safety

### T2-Medium Issues (1)
- [ ] internal_build_coupling

### Positive Patterns to Preserve
- ✅ Channel ownership documentation
- ✅ ReconfigureForBuild consolidation
- ✅ BuilderDependencies/BuilderState split
- ✅ AGENTS.md architectural contract
