# Kosh Go Style Guide
## The Definitive Reference for Production-Grade Go at Scale

> *"Write code that a stranger can understand at 2am during an incident."*

---

## Table of Contents

1. [Philosophy](#1-philosophy)
2. [Package Design](#2-package-design)
3. [Naming Conventions](#3-naming-conventions)
4. [Function & Method Design](#4-function--method-design)
5. [Options Pattern](#5-options-pattern)
6. [Error Handling](#6-error-handling)
7. [Interfaces](#7-interfaces)
8. [Concurrency](#8-concurrency)
9. [Context Usage](#9-context-usage)
10. [Memory & Performance](#10-memory--performance)
11. [Struct Design](#11-struct-design)
12. [Testing](#12-testing)
13. [Documentation](#13-documentation)
14. [Anti-Patterns to Eliminate](#14-anti-patterns-to-eliminate)
15. [File Organization](#15-file-organization)
16. [Dependency Injection](#16-dependency-injection)
17. [Build Constraints & Platform Code](#17-build-constraints--platform-code)
18. [Logging](#18-logging)
19. [Refactoring Checklist](#19-refactoring-checklist)

---

## 1. Philosophy

### The Four Laws of Kosh Go Code

**Law 1 — Explicit over implicit.**
If something could be ambiguous, make it explicit. A long clear name beats a short clever one every time. Future readers should never need to ask "what does this do?"

**Law 2 — Errors are values, not afterthoughts.**
Every error path is a first-class citizen. Handle it, wrap it with context, or explicitly ignore it with a named reason. Silent failure is the enemy.

**Law 3 — Concurrency is a sharp tool.**
Reach for goroutines only when the problem genuinely demands it. When you do use concurrency, the data ownership, synchronization invariants, and lifetime must be documented at the declaration site — not discoverable only by reading the implementation.

**Law 4 — The package boundary is a contract.**
Packages export what callers need, hide what they don't. Exported identifiers carry an implicit promise of stability. Internal details change freely; exported ones do not.

---

## 2. Package Design

### 2.1 Package Naming

Packages are named with a single lowercase word — no underscores, no mixed case, no stutter.

```go
// GOOD
package async
package cache
package retry
package fs

// BAD
package buildAsync
package cacheManager
package fs_utils
```

### 2.2 Package Cohesion

A package does one thing. If you find yourself writing `package utils` or `package helpers`, stop — those are symptoms of deferred decisions. Find the real abstraction.

```
builder/
  async/     — goroutine lifecycle patterns
  cache/     — BoltDB + content-addressed storage
  fs/        — filesystem abstractions and VFS
  retry/     — retry primitives
  pools/     — buffer pool management
  scheduler/ — build scheduler tokens
```

### 2.3 Import Grouping

Three groups, always in this order, separated by a blank line:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "sync"

    // 2. Third-party
    "github.com/spf13/afero"
    "go.etcd.io/bbolt"

    // 3. Internal (same module)
    "github.com/Kush-Singh-26/kosh/builder/fs"
    "github.com/Kush-Singh-26/kosh/builder/models"
)
```

Never mix groups. `goimports` enforces this automatically — run it.

### 2.4 Avoid Package-Level State

Package-level variables that mutate at runtime are hidden global state. They make code untestable and cause race conditions. Use dependency injection instead. The only acceptable package-level variables are:

- Compile-time constants
- Sync-once initialized caches (LRU, etc.) with explicit initialization functions
- `var ErrXxx = errors.New(...)` sentinel errors

```go
// ACCEPTABLE — sync.Once initialized, read-only after init
var (
    createdDirs     *lru.Cache[string, bool]
    createdDirsInit sync.Once
)

func getCreatedDirsCache() (*lru.Cache[string, bool], error) {
    createdDirsInit.Do(func() { /* ... */ })
    return createdDirs, createdDirsErr
}

// BAD — mutable global modified by multiple goroutines
var globalBuildCount int
```

---

## 3. Naming Conventions

### 3.1 Variables

Name variables for what they contain, not their type.

```go
// GOOD
ctx            context.Context
logger         *slog.Logger
taskQueue      chan T
dirtyFiles     map[string]bool
targetDir      string

// BAD
c              context.Context   // one letter (except loop indices)
l              *slog.Logger
ch             chan T             // ambiguous
m              map[string]bool
s              string            // tells you nothing
```

**Exception**: Loop indices (`i`, `j`, `k`), error values (`err`), and receiver names follow established Go convention.

### 3.2 Receivers

Receiver names are short (1-2 characters), consistent across all methods of a type, and derived from the type name.

```go
// GOOD
func (m *Manager) BatchCommit(...) error
func (m *Manager) GetPostByID(...) (*PostMeta, error)
func (tx *TxSync) TrackWrite(...) error
func (p *WorkerPool[T]) Start()

// BAD
func (manager *Manager) BatchCommit(...) error
func (this *Manager) GetPostByID(...) (*PostMeta, error)
func (self *WorkerPool[T]) Start()
```

### 3.3 Constants and Errors

Exported error variables use `Err` prefix. Unexported ones use `err` prefix only for local variables; package-level unexported errors use the `err` prefix too.

```go
// Exported sentinel errors
var ErrNoContent = fmt.Errorf("no content found in cache")

// Unexported sentinel (used within package only)
var errBucketMissing = errors.New("bucket not found")
```

Constants are `MixedCase`, not `ALL_CAPS` (that's C, not Go).

```go
const (
    WriteBufferSize     = 64 * 1024
    DefaultMemCacheTTL  = 5 * time.Minute
    MaxWorkers          = 32
)
```

### 3.4 Booleans

Boolean variables and functions returning booleans should read as a question.

```go
// GOOD
isCleanBuild   bool
isDev          bool
hasImages      bool
func IsAlwaysSyncPath(relPath string) bool
func (tx *TxSync) IsCommitted() bool

// BAD
cleanBuild     bool
devMode        bool
images         bool
```

### 3.5 Channel Naming

Channels are named for what they carry or signal, with a `Ch` or `Chan` suffix only when needed for clarity.

```go
assetsReady    <-chan struct{}   // signals readiness
discoveryChan  chan []ScannedAsset
errCh          chan error
taskQueue      chan T            // obvious without suffix
```

---

## 4. Function & Method Design

### 4.1 The Single Responsibility Principle

A function does one thing. If you need to write "and" to describe what a function does, split it.

```go
// BAD — doing too much
func processAndStoreAndNotify(post *PostMeta, notify bool) error

// GOOD — single responsibility
func processPost(post *PostMeta) (*ProcessedPost, error)
func storePost(processed *ProcessedPost) error
func notifyPostStored(postID string) error
```

### 4.2 Parameter Count

**Hard limit: 4 parameters.** When a function needs more than 4 parameters, use the Options pattern (see §5). This applies to constructors, exported functions, and any function called from more than one site.

```go
// GOOD — 4 or fewer params
func NewWorkerPool[T any](ctx context.Context, workers int, handler func(T) error) *WorkerPool[T]

func FireAndForget(ctx context.Context, logger *slog.Logger, operation string, fn func() error)

// REQUIRES OPTIONS PATTERN — 5+ params
func SyncVFS(ctx context.Context, srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool, isCleanBuild bool) error
// ^ This is at the limit; add one more field and it must become an options struct.
```

### 4.3 Return Values

Return the zero value and an error. Never return a non-nil error alongside a meaningful non-nil result unless the partial result is intentional and documented.

```go
// GOOD — zero value on error
func (m *Manager) GetPostByID(postID string) (*core.PostMeta, error) {
    result, err := getCachedItem[core.PostMeta](m.db, core.BucketPosts, []byte(postID))
    if err != nil {
        return nil, err  // zero value + error
    }
    return result, nil
}

// BAD — non-nil result on error
func parseConfig(data []byte) (*Config, error) {
    cfg := DefaultConfig()
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return cfg, err  // never return partial state on error
    }
    return cfg, nil
}
```

**Exception**: The functional-options pattern and builder patterns may return `self` for chaining, which is acceptable.

### 4.4 Named Return Values

Use named return values only for documentation clarity in short functions or when using `defer` to modify the return. Never use naked returns in functions longer than 5 lines.

```go
// ACCEPTABLE — named for documentation, explicit return
func splitPath(p string) (dir, file string) {
    dir = filepath.Dir(p)
    file = filepath.Base(p)
    return dir, file  // explicit, not naked
}

// BAD — naked return, unreadable
func doWork() (result int, err error) {
    result = compute()
    if result < 0 {
        err = errors.New("negative")
        return  // naked — what is being returned?
    }
    return  // naked again
}
```

### 4.5 Function Length

Functions should fit on one screen (roughly 50 lines). If a function grows beyond that, extract logically cohesive sections into helper functions with clear names.

```go
// Instead of one 120-line SyncVFS function:
func SyncVFS(ctx context.Context, srcFs afero.Fs, targetDir string, dirtyFiles map[string]bool, isCleanBuild bool) error {
    tx := fs.NewTxSync(slog.Default())
    defer func() {
        if !tx.IsCommitted() {
            tx.Rollback(ctx)
        }
    }()

    tasks, err := collectSyncTasks(srcFs, targetDir, dirtyFiles)  // extracted
    if err != nil {
        return err
    }

    if err := precreateDirectories(ctx, tasks); err != nil {  // extracted
        return err
    }

    if err := syncFiles(ctx, srcFs, tasks, isCleanBuild, tx); err != nil {  // extracted
        return err
    }

    tx.Commit()
    return nil
}
```

---

## 5. Options Pattern

This is the primary pattern for any function or constructor with more than 4 parameters, or any function whose signature is likely to evolve.

### 5.1 The Standard Form

```go
// Options struct — always named <Noun>Options
type SyncOptions struct {
    // Required fields listed first, no defaults
    SrcFs      afero.Fs
    TargetDir  string
    DirtyFiles map[string]bool

    // Optional fields with documented defaults
    IsCleanBuild bool          // default: false
    Logger       *slog.Logger  // default: slog.Default()
    Concurrency  int           // default: runtime.NumCPU()*2, capped at 32
}

// Constructor validates and applies defaults
func newSyncOptions(opts SyncOptions) SyncOptions {
    if opts.Logger == nil {
        opts.Logger = slog.Default()
    }
    if opts.Concurrency <= 0 {
        opts.Concurrency = min(runtime.NumCPU()*2, 32)
    }
    return opts
}

// Public function accepts options struct
func SyncVFS(ctx context.Context, opts SyncOptions) error {
    opts = newSyncOptions(opts)
    // ...
}
```

### 5.2 When to Use Options vs Plain Parameters

| Scenario | Use |
|---|---|
| ≤4 params, unlikely to change | Plain parameters |
| 5+ params | Options struct |
| ≤4 params but has optional fields | Options struct |
| Constructor for a service/manager | Options struct (always) |
| Internal helper called once | Plain parameters acceptable |

### 5.3 Embedding Required vs Optional

Required fields always have zero-value checks with a panic or error. Optional fields document their defaults in comments.

```go
type BuildAssetsOptions struct {
    // Required
    SrcFs    afero.Fs      // must not be nil
    Sink     ArtifactSink  // must not be nil
    SrcDir   string        // must not be empty
    DestDir  string        // must not be empty

    // Optional — defaults documented
    Minify           bool              // default: false
    CacheDir         string            // default: "" (no caching)
    Force            bool              // default: false
    Scheduler        BuildScheduler    // default: nil (no scheduling)
    OnWrite          func(string)      // default: nil (no callback)
    OnAssetProcessed func()            // default: nil (no callback)
}
```

### 5.4 Functional Options (When Appropriate)

Use functional options only for types that expose a fluent builder API or have a large number of optional overrides. The `WorkerPool.WithScheduler` pattern is a good example.

```go
// Good use of method chaining for optional configuration
pool := NewWorkerPool(ctx, workers, handler).
    WithScheduler(sched, TaskDefault).
    WithRetry(3)
```

Do **not** use functional options when a plain struct is clearer. The `func(o *Options)` pattern adds indirection without benefit for most cases.

---

## 6. Error Handling

### 6.1 Always Wrap with Context

Every error that crosses a function boundary should carry enough context to trace it to its origin without a debugger.

```go
// GOOD — each layer adds context
func (m *Manager) GetPostByPath(path string) (*PostMeta, error) {
    post, err := m.db.View(func(tx *bbolt.Tx) error { /* ... */ })
    if err != nil {
        return nil, fmt.Errorf("GetPostByPath(%q): %w", path, err)
    }
    return post, nil
}

// BAD — error loses context
func (m *Manager) GetPostByPath(path string) (*PostMeta, error) {
    post, err := m.db.View(func(tx *bbolt.Tx) error { /* ... */ })
    if err != nil {
        return nil, err  // where did this come from?
    }
    return post, nil
}
```

### 6.2 Sentinel Errors

Define sentinel errors at the package level using `errors.New`. Use `fmt.Errorf` only for dynamic messages. Use `%w` to wrap, `%v` to embed without wrapping.

```go
// Package-level sentinels
var (
    ErrNoContent    = errors.New("no content found in cache")
    ErrBucketMissing = errors.New("required bucket not found")
)

// Wrapping for propagation (caller can use errors.Is)
return fmt.Errorf("failed to read post %q: %w", postID, ErrNoContent)

// Embedding for final messages (not intended for programmatic checking)
return fmt.Errorf("schema version mismatch: got %d, want %d", current, expected)
```

### 6.3 Error Aggregation

Use `errors.Join` for collecting multiple errors in concurrent operations. Never swallow errors silently.

```go
// GOOD — collect all errors, return joined
func (p *WorkerPool[T]) Stop() error {
    close(p.taskQueue)
    p.wg.Wait()
    return errors.Join(p.errs...)
}

// BAD — only last error survives
var lastErr error
for _, task := range tasks {
    if err := process(task); err != nil {
        lastErr = err  // previous errors lost
    }
}
return lastErr
```

### 6.4 Explicit Error Ignoring

When an error genuinely can be ignored (cleanup paths, logging side-effects), use `_` with a comment explaining why.

```go
// GOOD — explicit ignore with reason
_ = f.Close()                    // best-effort; read already succeeded
_ = os.Remove(tmpPath)           // cleanup; if it fails, OS will clean up

// BAD — unexplained ignore
f.Close()      // return value silently discarded
os.Remove(tmp) // error silently discarded
```

For non-trivial ignores, use the existing `IgnoreError` helper:

```go
buildCtx.IgnoreError(sink.MkdirAll(dir), "directory may already exist")
```

### 6.5 Panic Policy

Panics are reserved for programmer errors (impossible states, violated invariants, failed preconditions during initialization). They are **never** used for runtime errors that can be handled.

```go
// ACCEPTABLE — programmer error during init
func getFontCache() *lru.Cache[string, *truetype.Font] {
    fontCacheOnce.Do(func() {
        var err error
        fontCache, err = lru.New[string, *truetype.Font](20)
        if err != nil {
            panic("failed to create font cache: " + err.Error())
            // This cannot fail for valid capacity; if it does, code is broken
        }
    })
    return fontCache
}

// NEVER — panic for runtime errors
func processPost(post *Post) {
    if post == nil {
        panic("nil post")  // BAD: return an error instead
    }
}
```

---

## 7. Interfaces

### 7.1 Interface Size

Interfaces should be small. One to three methods is ideal. An interface with ten methods is a concrete type wearing a costume.

```go
// GOOD — focused interfaces
type BuildScheduler interface {
    Acquire(ctx context.Context, task TaskType) error
    Release(task TaskType)
}

type SocialCardCache interface {
    GetSocialCardHash(path string) (string, error)
    SetSocialCardHash(path, hash string) error
    BatchSetSocialCardHashes(hashes map[string]string) error
}

// NEEDS SPLITTING — too many responsibilities
type BuildArtifactCache interface {
    GetHTMLContent(post *PostMeta) ([]byte, error)
    StoreHTML(content []byte) (string, error)
    StoreHTMLForPost(post *PostMeta, content []byte) error
    BatchCommit(posts []*PostMeta, records map[string]*SearchRecord, deps map[string]*Dependencies) error
    DeletePost(postID string) error
    MarkDirty(postID string)
    IsDirty(postID string) bool
    ClearDirty()
}
// Split into: HTMLStore, PostCommitter, DirtyTracker
```

### 7.2 Interface Placement

Interfaces belong **in the package that uses them**, not the package that implements them. This is the Go way and enables decoupling.

```go
// In builder/models/interfaces.go — the consumer defines the interface
type RenderService interface {
    RenderPage(path string, data PageData) error
    RegisterFile(path string)
    // ...
}
// The actual implementation is in builder/services/render/ — it doesn't know about this interface
```

### 7.3 Accept Interfaces, Return Concrete Types

Functions should accept interfaces (flexible) and return concrete types (predictable). The caller decides the abstraction level.

```go
// GOOD
func NewDiskSink(stagingDir, realOutputDir string) *DiskSink  // concrete return

func SyncVFS(ctx context.Context, srcFs afero.Fs, ...) error  // interface param
```

### 7.4 Interface Assertions

When an interface assertion is purely for compile-time checking, use the blank identifier form in a `var` declaration, not in `init` or at call sites.

```go
// In the implementation file, at package level
var _ models.RenderService = (*Service)(nil)     // compile-time check
var _ fs.ArtifactSink = (*DiskSink)(nil)         // compile-time check
```

---

## 8. Concurrency

### 8.1 Data Ownership

Every piece of shared mutable state must have a clear owner. Document who owns what, when ownership transfers, and what synchronizes access.

```go
// WorkerPool owns taskQueue — workers read, Submit writes
// errs is protected by mu — workers append, Stop reads after wg.Wait()
type WorkerPool[T any] struct {
    workers   int
    ctx       context.Context
    wg        sync.WaitGroup
    taskQueue chan T          // owned by pool; closed by Stop()
    handler   func(T) error
    stopped   atomic.Bool    // guards against double-stop
    errs      []error        // protected by mu
    mu        sync.Mutex
}
```

### 8.2 Mutex Discipline

- Lock the minimum scope necessary.
- Never call external functions (I/O, network, other locks) while holding a lock.
- Document what each mutex protects with a comment on the field.
- Prefer `RWMutex` when reads dominate.

```go
type Manager struct {
    mu      sync.RWMutex  // protects dirty
    dirty   map[string]bool

    memCache    *lru.Cache[string, *memoryCacheEntry]  // thread-safe, no mutex needed
}

func (m *Manager) MarkDirty(postID string) {
    m.mu.Lock()
    defer m.mu.Unlock()  // always use defer for unlock
    m.dirty[postID] = true
}

func (m *Manager) IsDirty(postID string) bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.dirty[postID]
}
```

### 8.3 Channel Patterns

Channels communicate data and signal events. Choose buffered vs unbuffered deliberately:

| Pattern | Buffer size | When |
|---|---|---|
| Signal (done, ready) | 0 (unbuffered) or 1 | Synchronization point |
| Error from goroutine | 1 | Goroutine must not block on send |
| Task queue | `workers * BufferSize` | Decouple producer from consumer |
| Fan-out | 0 | Backpressure is desired |

```go
// Error channel: always buffered size 1 to prevent goroutine leak
errCh := make(chan error, 1)

// Task queue: buffered to decouple submission from execution
taskQueue: make(chan T, workers*models.WorkerBufferSize)

// Done signal: unbuffered (synchronization point)
done := make(chan struct{})
```

### 8.4 The FireAndForget Contract

Background goroutines follow the `FireAndForget` pattern from `builder/async`. Never start a bare `go func()` for non-trivial work.

```go
// GOOD — uses standard pattern with panic recovery and logging
async.FireAndForget(ctx, logger, "cache commit", func() error {
    return cache.BatchCommit(posts, records, deps)
})

// BAD — bare goroutine with no panic recovery, no logging
go func() {
    cache.BatchCommit(posts, records, deps)
}()
```

### 8.5 WaitGroup Discipline

`wg.Add` is called before the goroutine starts. `wg.Done` is deferred as the first statement inside the goroutine.

```go
// GOOD
wg.Add(1)
go func() {
    defer wg.Done()  // first line, guaranteed to run
    doWork()
}()

// BAD — race: WaitGroup may reach zero before goroutine starts
go func() {
    wg.Add(1)         // too late
    defer wg.Done()
    doWork()
}()
```

### 8.6 Context Propagation

Every goroutine that runs longer than a single operation must respect context cancellation.

```go
func (p *WorkerPool[T]) worker() {
    defer p.wg.Done()
    for {
        select {
        case <-p.ctx.Done():   // always check context
            return
        case task, ok := <-p.taskQueue:
            if !ok {
                return
            }
            // process task
        }
    }
}
```

---

## 9. Context Usage

### 9.1 Context is Always First

Context is always the first parameter, always named `ctx`.

```go
func SyncVFS(ctx context.Context, srcFs afero.Fs, ...) error
func (m *Manager) GetPostByPath(ctx context.Context, path string) (*PostMeta, error)
```

### 9.2 Never Store Context in a Struct

Context is request-scoped. Storing it in a struct outlives the request and causes subtle bugs.

```go
// BAD
type WorkerPool[T any] struct {
    ctx context.Context  // BAD — context stored in struct
}

// The WorkerPool in this codebase is an exception because pool lifetime
// IS the context lifetime. Document this clearly when you make this tradeoff.
// For most types, pass ctx explicitly to each method.
```

### 9.3 Context Values

Use context values only for request-scoped metadata that crosses API boundaries (request IDs, trace IDs). Never use them to pass dependencies.

```go
// ACCEPTABLE — request-scoped metadata
ctx = context.WithValue(ctx, traceIDKey, traceID)

// BAD — dependency smuggling
ctx = context.WithValue(ctx, loggerKey, logger)  // pass logger explicitly instead
```

### 9.4 Context Cancellation Patterns

```go
// Timeout for external calls
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()  // always defer cancel to release resources

// Cancellation for background operations
ctx, cancel := context.WithCancel(ctx)
defer cancel()
```

---

## 10. Memory & Performance

### 10.1 Pool Usage

`sync.Pool` is used for objects that are allocated frequently and have a high GC cost. The canonical pattern:

```go
var bufPool = sync.Pool{
    New: func() any {
        b := make([]byte, 64*1024)
        return &b
    },
}

func doWork(data []byte) {
    bufPtr := bufPool.Get().(*[]byte)
    buf := *bufPtr
    defer bufPool.Put(bufPtr)   // return before any early returns

    // use buf
}
```

**Rules**:
- Always `Put` back — even on error paths. Use `defer`.
- Never store pointers to pool objects beyond the function scope.
- Reset the pooled object before `Put` if it contains state.

### 10.2 Pre-allocation

Pre-allocate slices and maps when the capacity is known or estimable.

```go
// GOOD — pre-allocated
tasks := make([]syncTask, 0, len(dirtyFiles)+len(models.AlwaysSyncPaths))
result := make(map[string]*PostMeta, len(postIDs))

// BAD — grows by reallocation
var tasks []syncTask
result := make(map[string]*PostMeta)
```

### 10.3 String Building

Use `strings.Builder` for building strings in loops. Never use `+=` in a loop.

```go
// GOOD
var sb strings.Builder
sb.Grow(estimatedLen)
for _, part := range parts {
    sb.WriteString(part)
}
return sb.String()

// BAD — O(n²) allocations
result := ""
for _, part := range parts {
    result += part
}
```

### 10.4 Avoid Unnecessary Allocations in Hot Paths

```go
// GOOD — stack-allocated key for map lookup
key := ssrType + ":" + inputHash  // only allocates if result is stored

// GOOD — reuse scratch buffer
scratch := p.scratch[:0]  // reset length, keep capacity

// BAD — allocation in tight loop
for _, post := range posts {
    key := fmt.Sprintf("path:%s", post.Path)  // fmt.Sprintf always allocates
    m.memCache.Get(key)
}

// GOOD — use string concatenation for simple cases
for _, post := range posts {
    key := "path:" + post.Path  // may optimize to stack
    m.memCache.Get(key)
}
```

### 10.5 Compression Tier Selection

Document compression tier decisions. This code already has a good pattern — preserve it:

```go
func determineCompression(size int) core.CompressionType {
    if size < models.RawThreshold {     // < 512 bytes: no compression (overhead > benefit)
        return core.CompressionNone
    }
    if size < models.FastZstdMax {      // < 64KB: fast zstd (good ratio, low CPU)
        return core.CompressionZstdFast
    }
    return core.CompressionZstdLevel3   // ≥ 64KB: level 3 (best ratio, acceptable CPU)
}
```

---

## 11. Struct Design

### 11.1 Field Ordering

Order fields to minimize padding (largest alignment first) and group related fields together with blank lines as section dividers.

```go
type Manager struct {
    // Core dependencies (pointers — 8 bytes each)
    db    *bbolt.DB
    store *store.Store

    // Configuration (strings, paths — typically 16 bytes each)
    basePath string
    cacheID  string

    // Synchronization (separate section)
    mu    sync.RWMutex
    dirty map[string]bool

    // In-memory cache layer (separate section)
    memCache    *lru.Cache[string, *memoryCacheEntry]
    memCacheTTL time.Duration

    // Reference counting
    refCount *gc.RefCountManager
}
```

### 11.2 Zero Value Usability

Structs should be usable in their zero state when possible, or the documentation must clearly state what initialization is required.

```go
// GOOD — zero value works
type BuildMetrics struct {
    StartTime time.Time  // zero = not started (valid state)
    // atomic fields are zero-initialized
    PostsProcessed atomic.Int64
}

// REQUIRES EXPLICIT INIT — document it
// TxSync must be created via NewTxSync(). Zero value is invalid.
type TxSync struct {
    mu      sync.Mutex
    backups map[string]string  // nil map panics on write
}
```

### 11.3 Embedded Types

Embed only when the embedding genuinely represents an "is-a" relationship or when promoting a subset of methods. Document embedded types.

```go
// GOOD — promotes interface methods
type statusWriter struct {
    http.ResponseWriter  // promoted: Header, Write, WriteHeader
    status int
}

// BAD — embedding for convenience, not semantics
type MyService struct {
    sync.Mutex  // callers should not call Lock/Unlock directly
}
```

---

## 12. Testing

### 12.1 Test File Conventions

- Test files are in the same package for white-box testing, or `package foo_test` for black-box.
- Use `package async` (not `package async_test`) for tests that need access to unexported identifiers.
- Use `package async_test` for integration-style tests that only use the public API.

The `rollback_test.go` pattern (external package) is correct for integration tests.

### 12.2 Table-Driven Tests

Table-driven tests are the default for any function with multiple input/output scenarios.

```go
func TestSanitizeSlug(t *testing.T) {
    tests := []struct {
        name  string   // always include a name field
        title string
        want  string
    }{
        {"simple words", "Hello World", "hello-world"},
        {"special chars", "Go: The Best", "go-the-best"},
        {"consecutive hyphens", "Multiple---Hyphens", "multiple-hyphens"},
        {"trim hyphens", "-Trim-", "trim"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := sanitizeSlug(tt.title)
            if got != tt.want {
                t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.title, got, tt.want)
            }
        })
    }
}
```

### 12.3 Test Helpers

Helper functions take `*testing.T` as the first argument and call `t.Helper()` as the first statement.

```go
func helperLogger(t *testing.T) *slog.Logger {
    t.Helper()
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func requireNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

### 12.4 Dependency Injection in Tests

Tests use real implementations with in-memory or temp-dir backends. Mocks are only introduced when the real implementation is impractical (external services, databases with no in-memory mode).

```go
// GOOD — real implementation with temp dir
func TestSyncVFS_Basic(t *testing.T) {
    srcFs := afero.NewMemMapFs()      // in-memory FS
    targetDir := t.TempDir()          // real temp dir, auto-cleaned
    // ...
}

// Mock interface for scheduler (external protocol, no real impl available)
type mockScheduler struct {
    acquire func(ctx context.Context, task scheduler.TaskType) error
    release func(task scheduler.TaskType)
}
```

### 12.5 Concurrency Tests

Tests for concurrent code use `sync.WaitGroup` or channels to synchronize, never `time.Sleep` as the primary wait mechanism.

```go
// GOOD — deterministic synchronization
var wg sync.WaitGroup
wg.Add(1)
FireAndForget(ctx, logger, "test", func() error {
    defer wg.Done()
    executed = true
    return nil
})
wg.Wait()

// BAD — flaky
FireAndForget(ctx, logger, "test", func() error {
    executed = true
    return nil
})
time.Sleep(100 * time.Millisecond)  // race condition waiting to happen
if !executed { ... }
```

`time.Sleep` is acceptable only as a secondary timeout guard:

```go
wg.Wait()  // primary
// time.Sleep used only in testutil.WaitForCondition as a polling fallback
```

---

## 13. Documentation

### 13.1 Package Documentation

Every package has a `doc.go` file or a doc comment on the first file. It explains the purpose of the package in 1-3 sentences and links to key types.

```go
// Package async provides standardized goroutine lifecycle patterns for
// background operations. It wraps go routines with panic recovery, structured
// logging, and optional error propagation via channels or callbacks.
//
// The primary entry point is FireAndForget for fire-and-forget operations,
// and WorkerPool for bounded concurrent task processing.
package async
```

### 13.2 Exported Identifiers

Every exported function, type, method, and constant has a doc comment. The comment begins with the identifier's name.

```go
// FireAndForget runs a function in a goroutine with standardized error handling.
// Errors are logged but don't propagate, suitable for background tasks like
// cache commits, social card generation, and other non-critical operations.
//
// The operation parameter names the task for log messages.
// The goroutine is protected against panics — a recovered panic is logged
// but does not propagate to the caller.
func FireAndForget(ctx context.Context, logger *slog.Logger, operation string, fn func() error) {
```

### 13.3 Non-Obvious Logic

Every non-obvious algorithmic decision or workaround gets a comment. The comment explains *why*, not *what* (the code shows what).

```go
// NoGrowSync and NoSync are safe for a build tool cache:
// the data is reproducible from source, so we trade durability for
// performance — especially important on Windows where fsync is slow.
opts.NoGrowSync = true
opts.NoSync = true
```

```go
// errCh is buffered size 1 to prevent goroutine leak:
// if the caller never reads from the channel, the goroutine
// can still complete and exit rather than blocking forever.
errCh := make(chan error, 1)
```

### 13.4 Error Contract Documentation

Functions with non-trivial error semantics document them explicitly.

```go
// BatchCommit atomically commits posts, search records, and dependencies.
//
// Error Contract:
//   - Returns error on BoltDB transaction failure or encoding error.
//   - Retry behavior: Safe to retry; idempotent within same build session.
//   - Thread safety: Concurrent calls are serialized via internal mutex.
//   - On error, no data is committed (all-or-nothing semantics).
func (m *Manager) BatchCommit(...) error {
```

---

## 14. Anti-Patterns to Eliminate

### 14.1 ❌ Magic Numbers

```go
// BAD
lruCache, err := lru.New[string, *memoryCacheEntry](1024)
time.Sleep(50 * time.Millisecond)
if len(hash) > 16 { ... }

// GOOD
const (
    memCacheMaxEntries = 1024
    workerShutdownTimeout = 50 * time.Millisecond
    hashDisplayLength = 16
)
lruCache, err := lru.New[string, *memoryCacheEntry](memCacheMaxEntries)
```

### 14.2 ❌ String Typing for Enumerations

```go
// BAD
func processSSR(ssrType string) { ... }  // what are valid values?
if ssrType == "d2" { ... }

// GOOD
type SSRArtifactType int
const (
    SSRTypeD2   SSRArtifactType = iota
    SSRTypeMath
)
func processSSR(ssrType SSRArtifactType) { ... }
```

### 14.3 ❌ Returning bool Instead of error

```go
// BAD
func processFile(path string) bool  // caller can't distinguish failure types

// GOOD
func processFile(path string) error  // caller gets full error context
```

### 14.4 ❌ Goroutine Without Lifetime

```go
// BAD — who owns this goroutine? when does it stop?
go func() {
    for {
        doBackgroundWork()
        time.Sleep(time.Second)
    }
}()

// GOOD — explicit lifetime, context-aware
go func() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            doBackgroundWork()
        }
    }
}()
```

### 14.5 ❌ Init Functions for Anything Beyond Package Registration

```go
// BAD — init() for runtime state
func init() {
    globalCache, _ = lru.New[string, any](1000)
}

// GOOD — lazy initialization with sync.Once
var (
    globalCache     *lru.Cache[string, any]
    globalCacheOnce sync.Once
)
func getCache() *lru.Cache[string, any] {
    globalCacheOnce.Do(func() {
        globalCache, _ = lru.New[string, any](1000)
    })
    return globalCache
}
```

### 14.6 ❌ Copying Structs with Mutexes

```go
// BAD — copies mutex, broken synchronization
opts := *existingManager  // Manager has sync.Mutex inside

// GOOD — use pointer or provide explicit Clone method
opts := existingManager.Clone()
```

### 14.7 ❌ Overusing `interface{}`/`any`

```go
// BAD — loses type safety
func store(key string, value any) { ... }

// GOOD — use typed interfaces or generics
func storeSSR[T SSRContent](key string, value T) { ... }

// ACCEPTABLE — when bridging typed/untyped boundaries (cache, serialization)
// Document exactly what types are expected
type DiagramCacheAdapter struct {
    local map[string]any  // values are: string | SSRThemePair
}
```

### 14.8 ❌ Defer in a Loop

```go
// BAD — defers accumulate until function returns, not iteration end
for _, path := range paths {
    f, _ := os.Open(path)
    defer f.Close()  // all closes happen at function end!
    process(f)
}

// GOOD — close in the loop body explicitly, or extract to a function
for _, path := range paths {
    if err := processFile(path); err != nil {
        return err
    }
}

func processFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()  // now defer is correct — function scope = iteration scope
    return process(f)
}
```

---

## 15. File Organization

### 15.1 One Concept Per File

Files are named for their primary concept. A file should contain one type and its methods, or one closely related group of functions.

```
cache/
  cache.go          — Manager type, Open/Close/initSchema
  cache_reads.go    — Manager read methods (Get*, List*)
  cache_writes.go   — Manager write methods (Store*, Batch*, Delete*)
  cache_queries.go  — Manager query methods (Stats, Social cards, Graph)
  cache_dirty.go    — Manager dirty tracking
  cache_batch_helpers.go — Internal batch operation types
  maintenance.go    — Clear, Rebuild
  types_aliases.go  — Re-exported types for backward compatibility
```

### 15.2 File Size Limits

Hard limit: 500 lines per file. If a file exceeds this, split by concept. A 1000-line file always contains at least two distinct concepts.

### 15.3 Generated Files

Generated files always have a `// Code generated by ... DO NOT EDIT.` header on line 1. They are **never** manually modified. Add a `go:generate` directive in the package that produces them.

```go
//go:generate msgp
package models
```

### 15.4 Build Constraint Files

Platform-specific files use build constraints, not `_linux.go`/`_windows.go` suffixes alone. The suffix is a hint; the constraint is the guarantee.

```go
//go:build linux && !wasm

package fs
```

---

## 16. Dependency Injection

### 16.1 Constructor Injection

Dependencies are injected through constructors, not set after the fact.

```go
// GOOD — dependencies explicit at construction
func NewManager(deps ManagerDependencies) *Manager {
    return &Manager{deps: deps}
}

type ManagerDependencies struct {
    Cfg      *config.Config
    Asset    asset.Service
    Render   render.Service
    Logger   *slog.Logger
    Metrics  *metrics.BuildMetrics
    SourceFs afero.Fs
}

// BAD — setter injection
mgr := &Manager{}
mgr.SetConfig(cfg)
mgr.SetRender(render)
mgr.SetLogger(logger)
```

### 16.2 Interface Parameters at Boundaries

Service and package boundaries accept interfaces; internal implementations are concrete.

```go
// Service boundary — accept interfaces
func RenderTags(opts TagOptions) error {
    // opts.Sink is ArtifactSink (interface)
    // opts.Render is RenderService (interface)
    // opts.Cache is SocialCardCache (interface)
}

// Internal function — concrete types fine
func syncSingleFileTask(ctx context.Context, srcFs afero.Fs, task syncTask, ...) error {
    // srcFs is afero.Fs (interface) because it genuinely needs to be swappable
    // task is syncTask (concrete struct) — internal implementation detail
}
```

### 16.3 Avoid Service Locators

Never use a global registry of services. Pass dependencies explicitly.

```go
// BAD — global service locator
var services = map[string]any{}
func RegisterService(name string, svc any) { services[name] = svc }
func GetService(name string) any { return services[name] }

// GOOD — explicit injection at construction
type Engine struct {
    Cfg  *config.Config
    Deps EngineDependencies
    Sink fs.ArtifactSink
    Ctx  *buildCtx.BuildContext
}
```

---

## 17. Build Constraints & Platform Code

### 17.1 Platform Abstraction

Platform-specific code is isolated to single-responsibility files. The public interface is defined once; platforms provide implementations.

```
fs/
  copy_fallback.go          — default (other platforms)
  copy_optimized_linux.go   — Linux copy_file_range syscall
  copy_optimized_windows.go — Windows CopyFileW syscall
  copy_optimized_wasm.go    — WASM stub

  flock.go          — FileLock type and public API
  flock_unix.go     — Unix flock syscall
  flock_windows.go  — Windows LockFileEx
  flock_wasm.go     — WASM no-op
```

### 17.2 WASM Stubs

WASM targets get stub implementations that return `errors.New("not supported")`. Never silently no-op an operation that has observable effects.

```go
//go:build wasm || js

func CopyFileInternal(src, dst string) error {
    return errors.New("CopyFileInternal not supported on wasm/js")
}
```

### 17.3 Feature Detection Over Platform Detection

Prefer runtime feature detection over compile-time platform detection when the capability genuinely varies within a platform.

```go
// Prefer — let the OS tell us
n, err := unix.CopyFileRange(...)
if err != nil {
    return StreamCopyFile(src, dst)  // fallback
}

// Avoid — assuming all Linux versions support copy_file_range
// (it was added in kernel 4.5 and requires same filesystem for reflinks)
```

---

## 18. Logging

### 18.1 Structured Logging Only

All logging uses `log/slog` with structured key-value pairs. `fmt.Println` and `log.Printf` are not used for logging.

```go
// GOOD
slog.Info("Syncing in-memory filesystem to disk", "clean_build", isCleanBuild)
slog.Error("BatchCommit failed", "count", len(postIDs), "ids", postIDs, "error", err)
slog.Warn("Cache integrity issues detected", "errors", len(errors))
slog.Debug("TxSync rename failed, retrying", "old", oldPath, "new", newPath, "error", err)

// BAD
fmt.Printf("Syncing filesystem, clean=%v\n", isCleanBuild)
log.Printf("ERROR: BatchCommit failed: %v", err)
```

### 18.2 Log Level Discipline

| Level | When |
|---|---|
| `Debug` | Detailed operational events useful for debugging; off by default |
| `Info` | Significant state transitions: build started, phase complete, file synced |
| `Warn` | Recoverable issues that may indicate problems: cache miss, retry |
| `Error` | Failures that impact correctness: file not written, cache corrupt |

Never log at `Error` for expected errors (file not found is a normal cache miss, not an error).

### 18.3 Logger Injection

The logger is injected as a dependency. Package-level `slog.Default()` is acceptable only in functions where injecting the logger would require refactoring across many call sites, and only as a fallback.

```go
// PREFERRED — injected logger
type TxSync struct {
    logger *slog.Logger
}

func NewTxSync(logger *slog.Logger) *TxSync {
    return &TxSync{logger: logger}
}

// ACCEPTABLE FALLBACK — when deep in a call stack with no injection path
slog.Info("Generated PWA Icon", "size", fmt.Sprintf("%dx%d", sz, sz))
```

---

## 19. Refactoring Checklist

Use this checklist when refactoring any file in the Kosh codebase.

### API Shape
- [ ] Does every function with 5+ parameters have an Options struct?
- [ ] Are all Options struct required fields documented and validated?
- [ ] Do functions return concrete types (not interfaces)?
- [ ] Do function signatures accept interfaces at boundaries?
- [ ] Are compile-time interface assertions present for major implementations?

### Error Handling
- [ ] Are all errors wrapped with `fmt.Errorf("...: %w", err)` at boundaries?
- [ ] Are all intentional error ignores explicit with `_` and a comment?
- [ ] Are sentinel errors defined at package level, not inline?
- [ ] Does no function return a non-nil result alongside a non-nil error?

### Concurrency
- [ ] Is every goroutine created via `async.FireAndForget` or `WorkerPool`?
- [ ] Does every mutex document what it protects?
- [ ] Are all channels buffered with a deliberate capacity?
- [ ] Does every `wg.Add` happen before `go func()`?
- [ ] Does every long-running goroutine check `ctx.Done()`?

### Code Quality
- [ ] Are there any magic numbers that should be named constants?
- [ ] Are there any `any`/`interface{}` usages that could be typed or generic?
- [ ] Is every `defer` outside a loop?
- [ ] Is every exported identifier documented?
- [ ] Is every non-obvious algorithm explained with a comment?

### Structure
- [ ] Is the file under 500 lines?
- [ ] Does each file contain a single concept?
- [ ] Are imports grouped correctly (stdlib / third-party / internal)?
- [ ] Does every package have a package doc comment?

### Testing
- [ ] Do tests use `t.Helper()` in helper functions?
- [ ] Are concurrent tests synchronized with WaitGroup/channels, not Sleep?
- [ ] Do table-driven tests have a `name` field and use `t.Run`?

---

## Appendix A: Quick Reference Card

```
Function params:     ≤ 4 → plain params | ≥ 5 → Options struct
Error wrapping:      fmt.Errorf("context: %w", err) at every boundary
Goroutines:          async.FireAndForget / WorkerPool, never bare go func()
Mutex comment:       // mu protects <field>
Channel buffer:      1 for error channels, 0 or N for task queues
Import groups:       stdlib | third-party | internal (blank line between)
File size:           ≤ 500 lines hard limit
Test helpers:        func helper(t *testing.T) { t.Helper(); ... }
Named returns:       only for defer-based mutation or 1-liner docs
```

## Appendix B: Naming Cheat Sheet

```
Types:           MixedCase                    — Manager, WorkerPool, TxSync
Interfaces:      Noun or NounVerber           — BuildScheduler, ArtifactSink
Errors:          ErrNoun                      — ErrNoContent, ErrBucketMissing
Constants:       MixedCase                    — MaxWorkers, WriteBufferSize
Boolean funcs:   Is/Has/Can/Should prefix     — IsCommitted, HasImages
Constructors:    New<Type>                    — NewManager, NewWorkerPool
Options structs: <Verb>Options                — SyncOptions, BuildAssetsOptions
Test helpers:    helper<Name>                 — helperLogger, helperTempDB
Receivers:       1-2 chars from type name     — m *Manager, tx *TxSync
```

---

*This guide is a living document. When you discover a new pattern that makes the codebase meaningfully better, add it here with an example from the actual codebase. The best style guides grow with the project.*
