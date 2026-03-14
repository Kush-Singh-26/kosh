# Comprehensive Codebase Audit - Detailed Remediation Plan

> Generated: March 2026  
> Repository: Kosh Static Site Generator  
> Version: v1.3.9

---

## Table of Contents

1. [Critical Bugs (P0 - Immediate Fix Required)](#1-critical-bugs-p0---immediate-fix-required)
2. [High Priority Issues (P1)](#2-high-priority-issues-p1)
3. [Performance Optimizations (P2)](#3-performance-optimizations-p2)
4. [Code Redundancies (P3)](#4-code-redundancies-p3)
5. [Architectural Improvements (P4)](#5-architectural-improvements-p4)
6. [Low Severity Issues (P5)](#6-low-severity-issues-p5)
7. [Implementation Roadmap](#7-implementation-roadmap)

---

## 1. Critical Bugs (P0 - Immediate Fix Required)

These bugs can cause data loss, silent build failures, or incorrect output. They should be fixed immediately.

### 1.1 Case Sensitivity Bug in Orphan Cleanup

**File:** `builder/run/cleanup.go`  
**Function:** `CleanupOrphans()`  
**Severity:** CRITICAL - Data Loss Risk

#### Issue Description

The orphan cleanup logic compares absolute file paths as raw strings:

```go
absWritten[filepath.ToSlash(abs)] = true
```

On Windows (the primary target platform), file systems are case-insensitive. If the `Sink` registers a path as `C:/Site/Image.png` but `os.Walk` yields `C:/site/image.png`, the string equality check fails. This causes the builder to incorrectly classify the file as an "orphan" and delete it, resulting in content loss.

#### Root Cause

- Case normalization is not performed before storing or comparing paths
- The code uses direct string comparison instead of case-insensitive comparison

#### How to Fix

1. **Normalize path casing on Windows before mapping:**
   - Create a helper function `normalizePathForComparison(path string) string` that converts paths to lowercase on Windows
   - Use `strings.EqualFold()` for path comparisons instead of direct string equality
   - Alternatively, use `filepath.EvalSymlinks()` to resolve and normalize paths

2. **Implementation approach:**
   ```go
   // Option A: Use strings.EqualFold for comparisons
   if !strings.EqualFold(abs, writtenPath) {
       // treat as orphan
   }
   
   // Option B: Normalize during storage
   normalizedPath := filepath.ToSlash(strings.ToLower(abs))
   absWritten[normalizedPath] = true
   ```

3. **Testing requirements:**
   - Add test with mixed-case paths on Windows
   - Verify no orphans detected when case differs but content is same

---

### 1.2 Unpropagated Errors on Asset Timeouts

**File:** `builder/services/render_service.go`  
**Functions:** `RenderPage`, `RenderIndex`, `RenderGraph`  
**Severity:** CRITICAL - Silent Build Failure

#### Issue Description

In each render function, the service waits for the `assetsReady` channel via a `select` statement with a 30-second timeout:

```go
select {
case <-assetsReady:
    // proceed normally
case <-time.After(30 * time.Second):
    logger.Warn("esbuild timed out, rendering with potentially stale assets")
    // proceeds with potentially missing/stale asset map
}
```

If esbuild hangs or takes too long, the code logs a warning and proceeds to render HTML with potentially missing or stale asset maps. This silently produces a broken build instead of failing loudly.

#### Root Cause

- Timeout error is logged but not propagated
- Build continues with potentially incorrect assets
- No distinction between "assets missing" and "assets timed out"

#### How to Fix

1. **Replace warning with fatal error:**
   - Instead of logging a warning, return an error from the render service
   - Let the error propagate up to the build pipeline
   - This ensures the build fails explicitly rather than producing incorrect output

2. **Implementation approach:**
   ```go
   select {
   case <-assetsReady:
       // proceed normally
   case <-time.After(30 * time.Second):
       return fmt.Errorf("asset build timed out after 30s - esbuild may be hung")
   }
   ```

3. **Alternative: Configurable timeout with retry:**
   - Add configuration option for timeout duration
   - Implement retry logic for transient failures

4. **Testing requirements:**
   - Simulate slow esbuild and verify build fails
   - Verify error message is descriptive

---

### 1.3 Template Cache Stampede Defeat

**File:** `builder/renderer/template_cache.go`  
**Function:** `hasTemplatesChanged`  
**Severity:** CRITICAL - Cache Defeat Under Load

#### Issue Description

The TTL check uses a Compare-And-Swap (CAS) to prevent multiple goroutines from hitting the disk simultaneously. However, if a goroutine loses the CAS, it sleeps for 50ms and then evaluates:

```go
changed := len(tc.mtimes) > 0
```

Because `tc.mtimes` is populated during the first template load, this expression is almost always `true`. Any concurrent request that loses the CAS will incorrectly determine that templates *have changed*, defeating the cache under load.

#### Root Cause

- Logic error in cache stampede handling
- The `changed` flag is incorrectly determined after losing CAS

#### How to Fix

1. **Fix the cache stampede logic:**
   - After losing CAS, the goroutine should wait for the first goroutine to complete and then use the result
   - Do not re-evaluate `changed` - trust the first goroutine's result

2. **Implementation approach:**
   ```go
   // After CAS failure and sleep:
   changed, err := tc.checkTemplatesOnDisk()  // Wait for result, don't re-evaluate
   // Or better: use the cached mtimes from the first goroutine
   ```

3. **Alternative: Use singleflight:**
   - Use `golang.org/x/sync/singleflight` to ensure only one goroutine checks templates

4. **Testing requirements:**
   - Load test with concurrent template requests
   - Verify cache hit rate is high under concurrent load

---

### 1.4 Fallback Layout Execution Renders Empty Page

**File:** `builder/renderer/render_special.go`  
**Functions:** `RenderIndex`, `Render404`  
**Severity:** CRITICAL - UX/Display Bug

#### Issue Description

If the `index.html` or `404.html` templates are missing, the system falls back to executing the layout directly:

```go
layout.Execute(buf, data)
```

Standard Go HTML templates use `layout.html` as a shell containing a `{{ block "content" . }}` placeholder. If executed on its own without the specific sub-template attached, it will render an empty page shell with no inner content.

#### Root Cause

- Fallback logic assumes layout can be executed standalone
- Missing template handling is incorrect

#### How to Fix

1. **Do not fallback to layout-only execution:**
   - If template is missing, return an explicit error
   - Do not attempt to render partial layout

2. **Implementation approach:**
   ```go
   if template == nil {
       return fmt.Errorf("required template %s not found", templateName)
   }
   // Remove the fallback to layout.Execute
   ```

3. **Testing requirements:**
   - Remove index.html template and verify error is returned
   - Verify error message helps identify missing template

---

## 2. High Priority Issues (P1)

These issues can cause performance problems or incorrect behavior but may not cause immediate data loss.

### 2.1 Memory Bloat in Phase 1 Parsing

**File:** `builder/services/post_service.go`  
**Function:** `Process()`  
**Severity:** HIGH - Memory Pressure

#### Issue Description

During `Process()`, the `parsePool` aggregates every single rendered post's `htmlContent` (often large strings) into a single `readyToRender` slice before dispatching to Phase 2. For a large site (e.g., 5,000+ posts), keeping all HTML strings in memory simultaneously causes massive RSS memory spikes.

#### Root Cause

- Phase 1 and Phase 2 are tightly coupled with a large memory buffer
- No streaming between phases

#### How to Fix

1. **Stream HTML directly to disk/cache:**
   - Instead of aggregating in memory, stream each parsed post directly to cache/disk
   - Tightly couple Phase 1 and Phase 2 pipelines

2. **Implementation approach:**
   ```go
   // Instead of:
   readyToRender = append(readyToRender, renderedPost)
   
   // Use:
   err := cacheStore.Store(renderedPost.Path, renderedPost.HTML)
   if err != nil {
       return err
   }
   // Notify Phase 2 immediately
   phase2Chan <- renderedPost.Path
   ```

3. **Alternative: Batched dispatch:**
   - Process posts in batches (e.g., 100 posts at a time)
   - Reduce memory footprint while maintaining some parallelism

4. **Testing requirements:**
   - Profile memory usage with 1000+ posts
   - Verify memory is released between batches

---

### 2.2 Inefficient YAML Parsing in Scanner

**File:** `builder/services/scanner.go`  
**Function:** (YAML unmarshaling in scanner)  
**Severity:** HIGH - Performance

#### Issue Description

The scanner parses frontmatter using `yaml.Unmarshal` into an untyped `map[string]any` for every single file just to extract high-level metadata (`title`, `date`, `tags`). `gopkg.in/yaml.v3` is notoriously slow at dynamic map unmarshaling.

#### Root Cause

- Using full YAML unmarshaling for simple metadata extraction
- No optimization for the common case of needing only a few fields

#### How to Fix

1. **Use a lightweight custom scanner:**
   - Parse only the required fields (title, date, tags)
   - Use regex or simple string extraction for common cases

2. **Implementation approach:**
   ```go
   // Extract frontmatter boundaries
   frontmatterRegex := regexp.MustCompile(`^---\n([\s\S]*?)\n---`)
   match := frontmatterRegex.FindSubmatch(content)
   
   // For common fields, use simple extraction
   // Only fall back to full YAML for complex/nested metadata
   ```

3. **Alternative: Strictly typed struct:**
   - Define a minimal struct for required fields
   - Use `yaml.Unmarshal` with specific type

4. **Testing requirements:**
   - Benchmark with 1000+ markdown files
   - Compare parsing time before/after

---

### 2.3 Global Mutex Contention in Hot Worker Loops

**File:** `builder/services/post_service.go`  
**Function:** Worker functions in `parsePool`  
**Severity:** HIGH - Performance / Scalability

#### Issue Description

Inside the `parsePool` worker function, the code constantly locks and unlocks several global mutexes (`indexedMu`, `batchMu`, `renderMu`, `phase1Mu`) to append results to slices. As the `workerCount` scales up (especially on high core-count CPUs), these shared mutexes become severe contention points, neutralizing the benefits of parallelism.

#### Root Cause

- Shared mutable state requires locking
- No per-worker local accumulation

#### How to Fix

1. **Use per-worker local slices:**
   - Each worker accumulates results in a local slice
   - Only lock when merging local results to global state

2. **Implementation approach:**
   ```go
   // Worker function
   localResults := make([]RenderedPost, 0, batchSize)
   
   for post := range jobs {
       // Process post
       localResults = append(localResults, result)
   }
   
   // Merge once at the end
   globalMu.Lock()
   readyToRender = append(readyToRender, localResults...)
   globalMu.Unlock()
   ```

3. **Alternative: Buffered channels:**
   - Use buffered channels instead of mutexes
   - Single consumer goroutine collects results

4. **Testing requirements:**
   - Benchmark with high worker counts (16, 32, 64)
   - Measure lock wait time

---

### 2.4 Synchronous Watch Mode Blocking

**File:** `builder/run/incremental.go`  
**Function:** `BuildChanged` handler  
**Severity:** HIGH - Stability

#### Issue Description

The `BuildChanged` handler locks `b.buildMu.Lock()` synchronously. If a flurry of file changes triggers fsnotify events, synchronous blocking can block the OS event loop, causing the fsnotify buffer to overflow and drop subsequent events.

#### Root Cause

- Synchronous lock acquisition in event handler
- No debouncing at the fsnotify level

#### How to Fix

1. **Use non-blocking lock with queue:**
   - Try to acquire lock, if blocked, queue the build request
   - Process queue sequentially

2. **Implementation approach:**
   ```go
   func (b *Builder) BuildChanged(paths []string) {
       select {
       case b.buildQueue <- paths:
           // Queued successfully
       default:
           // Queue full, merge with existing queue
           // or drop (depending on requirements)
       }
   }
   
   // Separate goroutine processes queue
   func (b *Builder) processQueue() {
       for paths := range b.buildQueue {
           b.buildMu.Lock()
           // ... build
           b.buildMu.Unlock()
       }
   }
   ```

3. **Testing requirements:**
   - Trigger rapid file changes
   - Verify no events are dropped

---

### 2.5 Debounce Deadlock Risk

**File:** `builder/run/incremental.go`  
**Function:** `regenerateSearchIndex`  
**Severity:** HIGH - Stability / Deadlock

#### Issue Description

The `regenerateSearchIndex` uses a standard `time.AfterFunc` debouncer that immediately requests `b.buildMu.Lock()`. If a full build is currently running and occupying the lock, the timer thread blocks indefinitely.

#### Root Cause

- Timer goroutine attempts to acquire lock held by main build goroutine
- No timeout on lock acquisition in debouncer

#### How to Fix

1. **Use try-lock or queue-based approach:**
   - Use `buildMu.TryLock()` if available
   - Or queue the request and let main goroutine process

2. **Implementation approach:**
   ```go
   func (b *Builder) regenerateSearchIndex() {
       b.searchIndexCh <- struct{}{}
   }
   
   func (b *Builder) processSearchIndex() {
       select {
       case <-b.searchIndexCh:
           // Wait for lock with timeout
           done := make(chan struct{})
           go func() {
               b.buildMu.Lock()
               close(done)
           }()
           
           select {
           case <-done:
               // Proceed
           case <-time.After(10 * time.Second):
               // Retry or skip
           }
       }
   }
   ```

3. **Testing requirements:**
   - Trigger rapid rebuilds
   - Verify no deadlock occurs

---

## 3. Performance Optimizations (P2)

These optimizations improve performance but are not correctness issues.

### 3.1 Search Result Sorting Overhead

**File:** `builder/search/engine.go`  
**Function:** `PerformSearch`  
**Severity:** MEDIUM - Performance

#### Issue Description

`slices.SortFunc` is used to sort the entire results array before it truncates the set to the top 40. For a very large index size, sorting thousands of matches rather than using a fixed-size Min-Heap (or partial sort) introduces an `O(N log N)` bottleneck on the front-end WASM engine.

#### How to Fix

1. **Use partial sort or heap:**
   - Use `slices.PartialSort` to get top K elements
   - Or use a Min-Heap of size K

2. **Implementation approach:**
   ```go
   // Partial sort - O(N log K) instead of O(N log N)
   slices.PartialSort(results, min(40, len(results)), func(a, b *SearchResult) int {
       return b.Score - a.Score
   })
   
   // Or heap approach
   h := &resultHeap{}
   for _, r := range results {
       if h.Len() < 40 {
           heap.Push(h, r)
       } else if r.Score > (*h)[0].Score {
           heap.Pop(h)
           heap.Push(h, r)
       }
   }
   ```

---

### 3.2 Fuzzy Search Short-Word Performance Degradation

**File:** `builder/search/fuzzy.go`  
**Function:** `FuzzyExpandWithNgrams`  
**Severity:** MEDIUM - Performance

#### Issue Description

The required minimum trigram overlap is calculated as `minScore := len(trigrams) / 2`. If a user types a 3-character word (which generates exactly 1 trigram), `minScore` resolves to `0`. Consequently, `if score >= minScore` behaves as `score >= 0`, forcing the engine to fallback and test the Levenshtein distance against *every single term* in the `ngramIndex`.

#### How to Fix

1. **Set minimum score to 1:**
   ```go
   minScore := max(1, len(trigrams)/2)
   ```

2. **Alternative: Use adaptive threshold based on query length**

---

### 3.3 PostCache Redundancy in Search

**File:** `builder/search/engine.go`  
**Function:** `PerformSearch`  
**Severity:** MEDIUM - Performance / Memory

#### Issue Description

In `PerformSearch`, there is a local `postCache` map used to avoid looking up records multiple times in `index.Posts`. However, `index.Posts` is already an in-memory map holding `models.PostRecord` values. Copying a struct from one map to another during a single query does not yield a lookup speedup and instead causes unnecessary memory allocations and copying overhead.

#### How to Fix

1. **Remove postCache:**
   - Use `index.Posts[postID]` directly
   - If caching is needed, cache pointer not struct

2. **Implementation:**
   ```go
   // Remove:
   postCache := make(map[string]models.PostRecord)
   
   // Use directly:
   post, ok := index.Posts[postID]
   ```

---

### 3.4 D2 Goroutine Overhead

**File:** `builder/parser/unified.go`  
**Function:** `renderD2Blocks`  
**Severity:** MEDIUM - Performance

#### Issue Description

The code iterates over all D2 blocks found in a document and unconditionally launches a goroutine for each one. While `t.D2Group.Do` (Singleflight) prevents duplicate *processing* of the same hash, launching redundant goroutines that simply block waiting on the singleflight group adds unnecessary scheduler overhead.

#### How to Fix

1. **Deduplicate hashes locally:**
   ```go
   // Collect unique hashes first
   uniqueHashes := make(map[string]bool)
   for _, block := range d2Blocks {
       uniqueHashes[block.Hash] = true
   }
   
   // Only launch goroutines for unique hashes
   var wg sync.WaitGroup
   for hash := range uniqueHashes {
       wg.Add(1)
       go func(h string) {
           defer wg.Done()
           t.D2Group.Do(h, func() (interface{}, error) {
               // process
           })
       }(hash)
   }
   wg.Wait()
   ```

---

### 3.5 Inefficient AST Text Extraction in TOC

**File:** `builder/parser/unified.go`  
**Function:** `Transform`  
**Severity:** MEDIUM - Performance

#### Issue Description

During the primary `ast.Walk` phase, when encountering an `ast.KindHeading`, the code spins up a *nested* `ast.Walk` to extract the heading text for the TOC. The parser traverses the inner text nodes of headings multiple times—once for the nested TOC extraction, and again as the primary walker continues its sweep to extract `plainText` for search indexing.

#### How to Fix

1. **Maintain state context during single sweep:**
   ```go
   // Add context to track we're in a heading
   type TransformContext struct {
       inHeading bool
       headingText strings.Builder
       // ...
   }
   
   // During walk:
   case *ast.Heading:
       ctx.inHeading = true
       ctx.headingText.Reset()
       // Walk children - collect text into headingText
       // Also collect into plainText for search
       ctx.inHeading = false
   ```

---

## 4. Code Redundancies (P3)

These are code duplication issues that hurt maintainability.

### 4.1 Massive Duplication in Render Pipelines

**Files:** `builder/renderer/render_page.go`, `builder/renderer/render_special.go`  
**Severity:** MEDIUM - Maintainability

#### Issue Description

`RenderPage`, `RenderIndex`, `RenderGraph`, and `Render404` contain identical 40-line blocks of execution logic (Acquire buffer, execute template, apply legacy regex `ProcessHTMLBytes`, apply minification, and perform stream write via `Sink`).

#### How to Fix

1. **Extract to unified helper:**
   ```go
   func (r *Renderer) renderGenericTemplate(
       ctx context.Context,
       templateName string,
       data interface{},
       sink utils.ArtifactSink,
   ) error {
       // Common logic:
       // 1. Acquire buffer from pool
       // 2. Execute template
       // 3. Apply ProcessHTMLBytes
       // 4. Apply minification
       // 5. Stream write via Sink
   }
   ```

2. **Testing requirements:**
   - Verify all render paths produce identical output
   - Add integration test for each template type

---

### 4.2 HasTagNormalized Wrapper

**File:** `builder/search/engine.go`  
**Severity:** LOW - Maintainability

#### Issue Description

`HasTagNormalized` merely wraps `slices.Contains`, adding redundant indirection.

#### How to Fix

1. **Remove wrapper and use `slices.Contains` directly**

---

### 4.3 Legacy Tokenize Function

**File:** `builder/search/analyzer.go`  
**Severity:** LOW - Maintainability

#### Issue Description

The package exposes a legacy `Tokenize(text string)` function which allocates a new string slice of tokens. Meanwhile, the pipeline correctly uses `TokenizeWithUnicodeInto` internally.

#### How to Fix

1. **Deprecate and remove legacy function:**
   ```go
   // Deprecated: Use TokenizeWithUnicodeInto for better performance
   func Tokenize(text string) []string {
       // Keep for backward compatibility or remove
   }
   ```

---

### 4.4 Pool Implementations Could Use Generics

**File:** `builder/utils/pools.go`  
**Severity:** LOW - Code Style

#### Issue Description

Five nearly identical pool implementations exist: `BufferPool`, `StringBuilderPool`, `BufioWriterPool`, `BufioReaderPool`, `ByteSlicePool`.

#### How to Fix

1. **Create generic pool:**
   ```go
   type Pool[T any] struct {
       pool sync.Pool
       new func() T
   }
   
   func NewPool[T any](newFn func() T) *Pool[T] {
       return &Pool[T]{
           pool: sync.Pool{
               New: func() interface{} { return newFn() },
           },
           new: newFn,
       }
   }
   ```

---

## 5. Architectural Improvements (P4)

These are larger architectural changes that improve code quality.

### 5.1 Asset Map Mutability Issue

**File:** `builder/renderer/renderer.go`  
**Function:** `GetAssets()`  
**Severity:** HIGH - Safety

#### Issue Description

`GetAssets()` dereferences and returns the actual underlying `map[string]string` from `atomic.Pointer`. Because maps are reference types in Go, any downstream caller modifying this map inadvertently mutates the shared global cache state.

#### How to Fix

1. **Return a clone:**
   ```go
   func (r *Renderer) GetAssets() map[string]string {
       assets := r.assets.Load()
       if assets == nil {
           return nil
       }
       // Return copy to prevent mutation
       result := make(map[string]string, len(*assets))
       for k, v := range *assets {
           result[k] = v
       }
       return result
   }
   ```

2. **Alternative: Use read-only interface**
   ```go
   type AssetMap interface {
       Get(key string) (string, bool)
       List() []string
   }
   ```

---

### 5.2 Atomic Write Collision Risk

**File:** `builder/utils/sync.go`  
**Severity:** MEDIUM - Reliability

#### Issue Description

Current temp file naming uses `time.Now().UnixNano()`:
```go
tmpPath := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
```

Multiple concurrent writes in the same nanosecond could collide.

#### How to Fix

1. **Add PID:**
   ```go
   tmpPath := path + ".tmp-" + fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
   ```

2. **Or use UUID:**
   ```go
   tmpPath := path + ".tmp-" + uuid.New().String()
   ```

---

### 5.3 Msgp Ignore Block

**File:** `builder/models/models.go`  
**Severity:** LOW - Architecture

#### Issue Description

The `//msgp:ignore` directive lists almost all structs in the file. Any new un-ignored struct placed in this file will automatically have serialization code generated.

#### How to Fix

1. **Isolate serialization structs:**
   - Move `IndexedPost`, `SearchIndex`, `PostRecord` to `models_search.go`
   - Keep other models separate

---

### 5.4 Double Stat on Directory Routes

**File:** `internal/server/server.go`  
**Severity:** LOW - Performance

#### Issue Description

Explicit logic intercepting `.IsDir()` to manually route to `index.html` results in a double `os.Stat` call.

#### How to Fix

1. **Optimize with single stat:**
   - Use `os.Lstat` followed by conditional follow
   - Or cache directory checks

---

### 5.5 TxSync Lock Contention

**File:** `builder/utils/tx_sync.go`  
**Severity:** MEDIUM - Scalability

#### Issue Description

With parallel file sync, the `TrackWrite` mutex becomes a contention point for large numbers of files.

#### How to Fix

1. **Batch operations:**
   ```go
   func (tx *TxSync) TrackWriteBatch(paths []string) error {
       tx.mu.Lock()
       defer tx.mu.Unlock()
       for _, p := range paths {
           // track
       }
       return nil
   }
   ```

2. **Or use lock-free approaches**

---

### 5.6 WASM Payload Size Optimization

**Files:** `cmd/search/main.go`, `internal/server/server.go`  
**Severity:** MEDIUM - Performance

#### Issue Description

The WASM client module fetches raw compressed bytes and decompresses them entirely in Go using `github.com/andybalholm/brotli`. This bloats the binary with decompression code.

#### How to Fix

1. **Serve pre-compressed files with proper headers:**
   - Create `search.bin` and `search.bin.br`
   - Set `Content-Encoding: br` header
   - Let browser's Fetch API decompress
   - Remove brotli package from WASM

---

## 6. Low Severity Issues (P5)

### 6.1 Image Cache Check-Then-Act Race

**File:** `builder/utils/fs_copy.go:87`  
**Severity:** LOW

The check-then-act pattern with separate atomic operations is technically racy, but the race window is extremely small.

**How to Fix:** Accept as-is, or add mutex protection.

---

### 6.2 ParallelWalk Goroutine Leak Edge Case

**File:** `builder/utils/walk.go:113`  
**Severity:** VERY LOW

If `tasks` channel is full and context is cancelled simultaneously, goroutine may not send.

**How to Fix:** Accept as-is.

---

### 6.3 WorkerPool Silent Task Drop

**File:** `builder/utils/worker_pool.go:70`  
**Severity:** LOW

If scheduler acquisition fails, task is silently dropped.

**How to Fix:** Add logging or return error.

---

### 6.4 Unused Slugify Function

**File:** `builder/utils/formatting.go`  
**Severity:** VERY LOW

Verify with grep and remove if unused.

---

## 7. Implementation Roadmap

### Phase 1: Critical Bugs (Week 1)

| Task | Owner | Estimated Effort |
|------|-------|------------------|
| Fix case sensitivity orphan cleanup | TBD | 2 hours |
| Fix asset timeout error propagation | TBD | 1 hour |
| Fix template cache stampede logic | TBD | 3 hours |
| Fix fallback layout empty render | TBD | 1 hour |

### Phase 2: High Priority (Week 2-3)

| Task | Owner | Estimated Effort |
|------|-------|------------------|
| Stream Phase 1 to Phase 2 | TBD | 8 hours |
| Optimize YAML scanner | TBD | 4 hours |
| Fix global mutex contention | TBD | 6 hours |
| Fix watch mode blocking | TBD | 4 hours |
| Fix debounce deadlock | TBD | 2 hours |

### Phase 3: Performance (Week 4-5)

| Task | Owner | Estimated Effort |
|------|-------|------------------|
| Implement partial sort for search | TBD | 2 hours |
| Fix fuzzy search minScore | TBD | 1 hour |
| Remove redundant postCache | TBD | 1 hour |
| Deduplicate D2 goroutines | TBD | 2 hours |
| Optimize AST TOC extraction | TBD | 4 hours |

### Phase 4: Redundancies (Week 6)

| Task | Owner | Estimated Effort |
|------|-------|------------------|
| Extract generic render helper | TBD | 4 hours |
| Remove HasTagNormalized wrapper | TBD | 30 minutes |
| Deprecate legacy Tokenize | TBD | 1 hour |
| Consider generic pools | TBD | 4 hours (optional) |

### Phase 5: Architecture (Week 7-8)

| Task | Owner | Estimated Effort |
|------|-------|------------------|
| Fix asset map mutability | TBD | 2 hours |
| Add PID to temp file naming | TBD | 1 hour |
| Isolate msgp structs | TBD | 2 hours |
| Optimize directory stat | TBD | 2 hours |
| Batch TxSync operations | TBD | 4 hours |
| WASM payload optimization | TBD | 6 hours |

---

## Appendix: Testing Checklist

### Critical Bug Testing

- [ ] Test orphan cleanup with mixed-case paths on Windows
- [ ] Verify build fails on esbuild timeout
- [ ] Load test template cache under concurrent requests
- [ ] Verify error on missing index/404 templates

### Performance Testing

- [ ] Profile memory with 1000+ posts
- [ ] Benchmark YAML parsing time
- [ ] Benchmark search with large result sets
- [ ] Profile mutex contention with high worker counts

### Integration Testing

- [ ] Full build with 500+ posts
- [ ] Dev mode with rapid file changes
- [ ] Clean build and verify output
- [ ] Search functionality verification

---

## Notes

- This plan assumes Go 1.26+ and existing test infrastructure
- Some fixes may require coordination (e.g., Phase 1 and Phase 2 changes)
- Testing is critical - do not skip testing phases
- Consider benchmarking before/after for performance changes

---

*End of Plan*
