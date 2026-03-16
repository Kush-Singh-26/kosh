# CacheService Composite Interface Refactor Plan

**Goal:** Split the 25+ method CacheService composite interface into focused sub-interfaces and inject only what each service needs.

**Current State:**
- CacheService embeds 5 sub-interfaces: PostCache, ContentCache, SocialCardCache, BuildArtifactCache, DirtyTracker
- Plus 2 lifecycle methods: Stats(), IncrementBuildCount(), Close()
- Total: 25+ methods in one composite interface
- Problem: Most callers only need a subset but must depend on the entire interface

**Target:** Interface segregation - each service depends only on what it actually uses.

---

## Phase 1: Analysis - Map Service Dependencies

### Services and Their Actual Cache Needs

| Service | Needs PostCache | Needs ContentCache | Needs SocialCard | Needs BuildArtifact | Needs DirtyTracker |
|---------|-----------------|-------------------|------------------|---------------------|-------------------|
| PostService | ✅ (read post metadata) | ✅ (store HTML) | ✅ (social cards) | ❌ | ✅ (mark dirty) |
| RenderService | ❌ | ✅ (read HTML) | ❌ | ❌ | ❌ |
| AssetService | ❌ | ❌ | ❌ | ✅ (graph/wasm hashes) | ❌ |
| Scanner | ✅ (read-only) | ❌ | ❌ | ❌ | ❌ |
| Site-wide generators | ❌ | ❌ | ❌ | ✅ (graph/wasm) | ❌ |

### Current Interface Structure

```go
// Composite interface (TO BE REMOVED)
type CacheService interface {
    PostCache
    ContentCache
    SocialCardCache
    BuildArtifactCache
    DirtyTracker
    Stats() (*cache.CacheStats, error)
    IncrementBuildCount() error
    Close() error
}

// Focused sub-interfaces (KEEP THESE)
type PostCache interface { ... }           // 6 methods
type ContentCache interface { ... }        // 7 methods
type SocialCardCache interface { ... }     // 3 methods
type BuildArtifactCache interface { ... }  // 4 methods
type DirtyTracker interface { ... }        // 3 methods
```

---

## Phase 2: Design New Dependency Injection Pattern

### Option A: Multiple Specific Interfaces (RECOMMENDED)

Each service constructor takes only the interfaces it needs:

```go
// PostService needs: PostCache + ContentCache + SocialCardCache + DirtyTracker + lifecycle
type PostServiceCacheDeps interface {
    PostCache
    ContentCache
    SocialCardCache
    DirtyTracker
    Stats() (*cache.CacheStats, error)
    IncrementBuildCount() error
    Close() error
}

// RenderService needs: ContentCache only
type RenderServiceCacheDeps interface {
    ContentCache
}

// AssetService needs: BuildArtifactCache only
type AssetServiceCacheDeps interface {
    BuildArtifactCache
}

// Scanner needs: PostCache (read-only)
type ScannerCacheDeps interface {
    PostCache
}
```

**Pros:**
- Clear interface segregation
- Each service explicitly declares its needs
- Easy to test with minimal mocks

**Cons:**
- Need to define new composite interfaces for services with multiple needs

### Option B: Direct Sub-Interface Injection

Pass multiple sub-interfaces as separate constructor parameters:

```go
type PostServiceDependencies struct {
    PostCache      PostCache
    ContentCache   ContentCache
    SocialCache    SocialCardCache
    DirtyTracker   DirtyTracker
    CacheStats     StatsProvider  // new interface for Stats()
}
```

**Pros:**
- Maximum granularity
- Most explicit about dependencies

**Cons:**
- More constructor parameters
- Need new interface for lifecycle methods

### Decision: Option A (Multiple Specific Interfaces)

Define focused composite interfaces per service that match actual usage patterns.

---

## Phase 3: Implementation Steps

### Step 1: Define Service-Specific Cache Interfaces

**File: `builder/services/interfaces.go`**

Add these new interfaces after the existing sub-interfaces:

```go
// PostServiceCache provides cache operations needed by PostService
type PostServiceCache interface {
    PostCache
    ContentCache
    SocialCardCache
    DirtyTracker
    Stats() (*cache.CacheStats, error)
    IncrementBuildCount() error
    Close() error
}

// RenderServiceCache provides cache operations needed by RenderService
type RenderServiceCache interface {
    ContentCache
}

// AssetServiceCache provides cache operations needed by AssetService
type AssetServiceCache interface {
    BuildArtifactCache
}

// ScannerCache provides cache operations needed by Scanner
type ScannerCache interface {
    PostCache
}

// SiteGeneratorCache provides cache operations needed by site-wide generators
type SiteGeneratorCache interface {
    BuildArtifactCache
    PostCache  // for post list metadata
}
```

### Step 2: Update cacheServiceImpl to Implement All Interfaces

**File: `builder/services/cache_service.go`**

The existing `cacheServiceImpl` already implements all sub-interfaces, so it automatically implements all the new service-specific interfaces. No changes needed here.

### Step 3: Update PostServiceDependencies

**File: `builder/services/post_service.go`**

Change:
```go
type PostServiceDependencies struct {
    Cfg            *config.Config
    Cache          CacheService  // OLD
    // ...
}
```

To:
```go
type PostServiceDependencies struct {
    Cfg            *config.Config
    Cache          PostServiceCache  // NEW - focused interface
    // ...
}
```

### Step 4: Update RenderServiceDependencies

**File: `builder/services/render_service.go`** (if it has explicit deps)

Update to use `RenderServiceCache` interface.

### Step 5: Update Builder Wiring

**File: `builder/run/builder.go`**

Find where cache service is created and passed to services:

```go
// OLD
cacheSvc := services.NewCacheService(cacheManager, logger)
postSvc := services.NewPostService(services.PostServiceDependencies{
    Cache: cacheSvc,
    // ...
})

// NEW - same code works! cacheSvc implements PostServiceCache
// The interface is narrower but cacheServiceImpl satisfies it
```

The beauty of this refactor: **no changes needed in builder.go** because `cacheServiceImpl` implements all the new focused interfaces automatically.

### Step 6: Update Test Mocks

**File: `builder/services/post_service_test.go`**

The `mockCacheService` already implements all methods, so it automatically satisfies the new `PostServiceCache` interface. May need to update type assertions if any.

### Step 7: Remove Old CacheService Interface

**File: `builder/services/interfaces.go`**

Delete the composite `CacheService` interface definition:

```go
// DELETE THIS
type CacheService interface {
    PostCache
    ContentCache
    SocialCardCache
    BuildArtifactCache
    DirtyTracker
    Stats() (*cache.CacheStats, error)
    IncrementBuildCount() error
    Close() error
}
```

### Step 8: Update NewCacheService Return Type

**File: `builder/services/cache_service.go`**

Change:
```go
func NewCacheService(manager *cache.Manager, logger *slog.Logger) CacheService {
    return &cacheServiceImpl{...}
}
```

To:
```go
func NewCacheService(manager *cache.Manager, logger *slog.Logger) PostServiceCache {
    return &cacheServiceImpl{...}
}
```

Or better, return the concrete type and let Go infer the interface:
```go
func NewCacheService(manager *cache.Manager, logger *slog.Logger) *cacheServiceImpl {
    return &cacheServiceImpl{...}
}
```

---

## Phase 4: Verification

### Build Verification
```bash
go build ./...
```

### Test Verification
```bash
go test ./builder/services/... -count=1 -timeout 60s
go test ./builder/run/... -count=1 -timeout 120s
```

### Desloppify Verification
```bash
python -m desloppify plan resolve cache_service_wide_interface \
  --attest "I have actually split CacheService into focused service-specific interfaces..." \
  --note "Split 25+ method composite into PostServiceCache, RenderServiceCache, etc."

python -m desloppify review --prepare --dimensions abstraction_fitness --force-review-rerun
```

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking changes to external APIs | Low | High | No external APIs affected - internal only |
| Test failures | Medium | Low | Mocks already implement all methods |
| Builder wiring issues | Low | Medium | Go's structural typing means no changes needed |
| Incomplete interface coverage | Low | Low | cacheServiceImpl implements everything |

---

## Estimated Effort

- **Step 1 (Define interfaces):** 15 minutes
- **Step 2-4 (Update deps):** 30 minutes
- **Step 5-6 (Update wiring/tests):** 30 minutes
- **Step 7-8 (Cleanup):** 15 minutes
- **Verification:** 15 minutes

**Total: ~2 hours**

---

## Expected Score Impact

| Dimension | Current | Expected After | Delta |
|-----------|---------|----------------|-------|
| abstraction_fitness | 78.5% | 88-92% | +10-14 pts |
| design_coherence | 82.5% | 85-88% | +3-6 pts |
| strict score | 87.2% | 90-92% | +3-5 pts |

After this fix, re-run subjective reviews for remaining low dimensions:
- convention_outlier (78.5%)
- initialization_coupling (82.0%)

---

## Success Criteria

1. ✅ CacheService composite interface removed
2. ✅ All services use focused interfaces matching their actual needs
3. ✅ All tests pass
4. ✅ Build succeeds
5. ✅ Desloppify abstraction_fitness score improves to 88%+
6. ✅ No new issues introduced
