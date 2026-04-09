Style-Guide Audit Report

## `blogs/builder/orchestration/builder.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 1 | 1–35 | **§2.3 Import Grouping** | `"github.com/spf13/afero"` (third-party) is mixed into the same import group as all the internal `github.com/Kush-Singh-26/kosh/…` packages. There are only 2 groups (stdlib + mixed), instead of the required 3. | Medium |
| 2 | 37–41 | **§14.5 init()** | `init()` calls `minify.InitHTMLMinifier()` and `debug.SetGCPercent(gcPercent)`. Both are runtime-state setup, not package registration. | High |
| 3 | ~170–240 | **§4.5 Function Length** | `initServices()` is approximately 70 lines, exceeding the ~50-line hard limit. | Medium |
| 4 | ~245–330 | **§4.5 Function Length** | `newEngineWithConfigFs()` is approximately 85 lines. | Medium |

---

## `blogs/builder/orchestration/engine.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 5 | 1–43 | **§2.3 Import Grouping** | Four separate import groups instead of three. `"github.com/spf13/afero"` is isolated in its own block, then `"github.com/Kush-Singh-26/kosh/builder/cache"` is isolated in its own block, then the rest of internal imports — 4 groups total. | Medium |
| 6 | ~85–115 | **§3.2 Receiver Naming** | `Engine` type consistently uses receiver `b` throughout the entire codebase. `b` is not derived from `Engine` (presumably a legacy from when the type was named `Builder`). Should be `e`. | Low |
| 7 | ~270 | **§6.1 Error Wrapping** | In `createOutputDirectories`, `return err` returns the raw error from `b.Sink.MkdirAll(...)` without wrapping: no `fmt.Errorf("failed to create %s: %w", dir, err)`. | Medium |
| 8 | — | **§7.4 Interface Assertions** | No `var _ SomeSiteBuilderInterface = (*Engine)(nil)` compile-time assertion for the `Engine` type. | Low |

---

## `blogs/builder/orchestration/sitewide_wait.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 9 | 1–8 | **§2.3 Import Grouping** | `"golang.org/x/sync/errgroup"` (third-party) is placed in the same import group as the internal `github.com/Kush-Singh-26/kosh/…` packages. Groups are mixed. | Medium |

---

## `blogs/builder/orchestration/pipeline_pwa.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 10 | 1–14 | **§2.3 Import Grouping** | Four groups instead of three. `"golang.org/x/sync/errgroup"` is in its own block, internal packages in their own block, then `"github.com/spf13/afero"` alone at the end. `afero` and `errgroup` should share one third-party group. | Medium |
| 11 | ~95–108 | **§3.4 Boolean Naming** | Local variable `needsGeneration` doesn't use an `Is/Has/Can/Should` prefix. Should be `shouldGenerateIcons` or `requiresIconGeneration`. | Low |
| 12 | ~88–100 | **§6.4 Explicit Error Ignoring** | `writeGeneratedPWAIcons` silently discards `os.MkdirAll` and multiple `os.WriteFile` errors with `_ = …` but provides no comment explaining why the errors are safe to ignore. | Medium |

---

## `blogs/builder/orchestration/post_service_mock.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 13 | 1–9 | **§2.3 Import Grouping** | `"github.com/spf13/afero"` (third-party) is mixed into the same import group as the internal packages. | Medium |
| 14 | ~37 | **§3.1 Variable Naming** | Parameter `l *slog.Logger` in `ReconfigureWithReporter`. `l` is explicitly prohibited as a name for a logger. | Low |

---

## `blogs/builder/orchestration/cleanup.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 15 | ~41, 58, 71 | **§6.4 Explicit Error Ignoring** | `_ = os.Remove(path)` appears twice inside walk callbacks and `_ = filepath.Walk(...)` appears for the empty-directory walk — all without any comment explaining why the error is intentionally discarded. | Medium |

---

## `blogs/builder/orchestration/logger.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 16 | ~11 | **§2.4 Package-Level State** | `var rebuildLevel = slog.Level(slog.LevelWarn + 1)` is a mutable package-level variable. Because `slog.LevelWarn` is a typed integer constant, this expression is a constant expression and should be declared `const`, not `var`. | Medium |
| 17 | ~11 | **§14.1 Magic Numbers** | The literal `+ 1` in `slog.LevelWarn + 1` is an unexplained magic number. Should be a named constant, e.g. `const rebuildLevelOffset = 1`. | Low |
| 18 | ~10–40 | **§18.3 Logger Injection** | `DevLogChange`, `DevLogRebuild`, `DevLogSuccess`, `DevLogSkip`, `DevLogInfo`, `DevLogError` all call package-level `slog.Log(…)` which implicitly uses `slog.Default()`. These bypass any injected logger configured on the `Engine`. | Medium |
| 19 | ~85–190 | **§4.5 Function Length** | `Handle()` method is approximately 110 lines, nearly double the hard limit. | Medium |

---

## `blogs/builder/orchestration/health.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 20 | ~155–170, ~190–215, ~260–290 | **§18.3 Logger Injection** | `BuildHealthRegistry` calls `slog.Warn`, `slog.Debug`, `slog.Error`, `slog.Info` (all implicit `slog.Default()`) in `RecordSlowPhase`, `recordEvent`, and `LogSummary`. No logger is injected into the struct. | Medium |
| 21 | ~158, ~268 | **§8.6 Context Propagation** | `context.TODO()` is passed to `slog.Default().Enabled(context.TODO(), …)` in `RecordSlowPhase` and `LogSummary`. A real context should be propagated or the method should accept `ctx context.Context`. | Low |

---

## `blogs/builder/orchestration/site_wide.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 22 | ~23–120 | **§4.5 Function Length** | `setupSiteWideRendering()` including its inner `runSiteWide` closure is approximately 100 lines. | Medium |
| 23 | ~52 | **§10.2 Pre-allocation** | `var allTags []models.TagData` is appended to in a range over `cb.TagMap`. Since `len(cb.TagMap)` is known, this should be `make([]models.TagData, 0, len(cb.TagMap))`. | Low |

---

## `blogs/builder/orchestration/assets/manager.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 24 | 1–21 | **§2.3 Import Grouping** | `"github.com/spf13/afero"` and `"github.com/zeebo/xxh3"` (third-party) are placed in the same import group as the internal `github.com/Kush-Singh-26/kosh/…` packages. | Medium |
| 25 | ~62–140 | **§4.5 Function Length** | `SetupBuilding()` is approximately 80 lines, exceeding the limit. | Medium |

---

## `blogs/builder/assets/pipeline.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 26 | ~95–115 | **§4.2 Parameter Count** | `handleImageTask(ctx, c, task, opts, errMu, errs)` — 6 parameters. Hard limit is 4; 5+ requires an Options struct. | High |
| 27 | ~117–130 | **§4.2 Parameter Count** | `handleNonImageTask(c, task, opts, errMu, errs)` — 5 parameters. Exceeds the limit. | High |
| 28 | ~132–165 | **§4.2 Parameter Count** | `startImageWorkers(ctx, c, opts, numWorkers, errMu, errs)` — 6 parameters. | High |
| 29 | ~167–200 | **§4.2 Parameter Count** | `startNonImageWorkers(ctx, c, opts, numWorkers, errMu, errs)` — 6 parameters. | High |
| 30 | ~140, ~180 | **§18.3 Logger Injection** | `slog.Default()` is passed as the logger argument to `async.FireAndForgetWithCleanup` in both `startImageWorkers` and `startNonImageWorkers`. The pipeline has no injected logger field. | Medium |
| 31 | ~265–275 | **§6.3 Error Aggregation** | `errs []error` accumulates multiple worker errors but `CopyDirVFS` returns only `errs[0]` (first-wins). Should use `errors.Join(errs...)`. | High |

---

## `blogs/builder/assets/image_processing.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 32 | ~145 | **§3.1 Variable Naming** | `m := koshMinify.GetHTMLMinifier()` in `maybeMinifySVG` uses the single-letter `m`, which is not in the allowed set (loop indices `i/j/k`, `err`, receiver names). | Low |
| 33 | ~28–34 | **§14.7 any/interface{} overuse** | `func isNil(i any) bool` accepts `any` for a nil-check. This is a perfect candidate for a generic function: `func isNil[T any](v T) bool`. | Low |

---

## `blogs/builder/assets/image_cache.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 34 | 1–30 | **§2.3 Import Grouping** | `"github.com/Kush-Singh-26/kosh/builder/async"` (internal) is placed in the same import block as the third-party packages `lru`, `afero`, `xxh3`, and `errgroup`. Internal and third-party are mixed. | Medium |
| 35 | ~34 | **§2.4 Package-Level State** | `var criticalOriginalPNGs = map[string]struct{}{…}` is a mutable package-level map. It's not a constant, sync.Once cache, or `ErrXxx` variable. Should be a `const`-block of strings or kept read-only with a comment. | Medium |
| 36 | ~64 | **§2.4 Package-Level State** | `var convertedImagePaths sync.Map` is a mutable global that acts as shared state across concurrent goroutines. Not a sync.Once cache and not an `ErrXxx`. | Medium |
| 37 | ~155–165 | **§2.4 Package-Level State** | `var imageCacheWriter struct { ch chan …; once sync.Once }` is a mutable global struct used as a singleton background writer. | Medium |
| 38 | ~82 | **§6.4 Explicit Error Ignoring** | `c, _ := lru.NewWithEvict[imageCacheKey, []byte](maxItems, onEvict)` discards the error return from `lru.NewWithEvict` without a comment. | Low |
| 39 | ~170–195 | **§14.4 Goroutine Without Lifetime** | The goroutine started in `initImageCacheWriter` uses `context.Background()` and loops `for entry := range imageCacheWriter.ch`. The channel is **never closed** anywhere in the codebase, so this goroutine runs for the entire process lifetime with no cancellation or shutdown mechanism. | High |
| 40 | ~170–195 | **§8.6 Context Propagation** | The same goroutine uses `context.Background()` — no context cancellation is propagated, violating the requirement that every long-running goroutine checks `ctx.Done()`. | High |
| 41 | ~172, ~310 | **§18.3 Logger Injection** | `slog.Default()` used in `initImageCacheWriter`; `slog.Info("Cleaned up original images", …)` in `CleanupOriginalImages` uses `slog.Default()` implicitly. Neither function has an injected logger. | Medium |
| 42 | ~15.1 concept | **§15.1 One Concept Per File** | `image_cache.go` contains: the LRU `imageCache` type, the `atomicInt64` utility, the `convertedImagePaths` conversion-tracking sync.Map and its helpers, the `imageCacheWriter` background goroutine, and the `CleanupOriginalImages`/`deleteOriginals` cleanup functions — at least four distinct concerns. | Medium |

---

## `blogs/builder/assets/wasm.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 43 | 1–15 | **§2.3 Import Grouping** | `fspkg "github.com/Kush-Singh-26/kosh/builder/fs"` (internal) is placed in the same import group as the third-party packages `brotli`, `afero`, and `xxh3`. | Medium |
| 44 | ~24 | **§2.4 Package-Level State** | `var embeddedWasmHash = SearchWasmHash` is a mutable `var` initialized from a constant. Since `SearchWasmHash` is a `const string`, `embeddedWasmHash` could and should be a `const`. | Low |
| 45 | ~27 | **§2.4 Package-Level State** | `var wasmInitErr error` is a mutable global error placeholder that is never assigned anywhere visible, yet is checked in `loadWasmSource`. This is invisible mutable global state. | Medium |
| 46 | ~30–40, ~110, ~160, ~185 | **§14.3 Bool Return** | `CheckWASM()`, `CheckWASMFs()`, `CheckWASMFsWithSource()`, `DeployWASMFromFile()`, and `DeployWASMFromFileWithLevel()` all return `bool` for operations that can fail for multiple distinct reasons (I/O errors, decompression errors, write failures). An `error` return would be vastly more informative to callers. | High |
| 47 | ~200–210 | **§6.4 Explicit Error Ignoring** | In `prepareSourceWasm`, `_ = os.MkdirAll(...)` and `_ = os.WriteFile(...)` have no accompanying comments. | Low |
| 48 | ~30–220 | **§18.3 Logger Injection** | Pervasive use of package-level `slog.Info`, `slog.Warn`, `slog.Error` throughout `wasm.go` (e.g. in `loadWasmSource`, `ensureWasmDir`, `hasDeployedWasm`, `writeWasmOutputs`, `prepareSourceWasm`). None of these functions receive an injected logger. | Medium |

---

## `blogs/builder/services/asset/service.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 49 | ~20, ~26, ~68, ~73, ~78 | **§3.1 Variable Naming** | Parameter `ch` is used for channel parameters throughout (`WithAssetsReadySignal(ch chan struct{})`, `WithContentAssetsChannel(ch <-chan …)`, `SetAssetsReadySignal(ch chan struct{})`, `SetDiscoveryReady(ch chan struct{})`, `SetContentAssetsChannel(ch <-chan …)`). `ch` is explicitly prohibited as a channel name. | Low |
| 50 | ~87 | **§3.1 Variable Naming** | `ReconfigureWithReporter(r ui.Reporter, l *slog.Logger)` — parameter `l` is prohibited as a logger name. | Low |

---

## `blogs/builder/services/asset/interfaces.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 51 | ~26–40 | **§7.1 Interface Size** | `Service` interface has **11 methods** (`ReconfigureForBuild`, `SetMetrics`, `SetAssetsReadySignal`, `SetContentAssetsChannel`, `SetDiscoveryReady`, `ReconfigureWithReporter`, `Build`, `BuildWithOptions`, `DiscoveryReady`, `BuildForAssetChange`, `BuildForAssetChangeWithOptions`). The flag threshold is 6+; ideal is 1–3. | Medium |
| 52 | ~26–40 | **§3.1 Variable Naming** | Interface method signatures repeat `ch` for channel and `l` for logger (same as service.go above). | Low |
| 53 | — | **§7.4 Interface Assertions** | No `var _ Service = (*assetService)(nil)` compile-time assertion anywhere in the `asset` package. | Low |

---

## `blogs/builder/services/asset/build.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 54 | ~102 | **§3.4 Boolean Naming** | `buildEsbuildAssets(force bool)` — parameter `force` has no `Is/Has/Can/Should` prefix. Should be `shouldForce`. | Low |
| 55 | ~120 | **§3.4 Boolean Naming** | `BuildForAssetChangeWithOptions(ctx, forceImages bool)` — `forceImages` has no boolean prefix. Should be `shouldForceImages`. | Low |
| 56 | ~23 | **§3.4 Boolean Naming** | `BuildWithOptions(ctx, skipImages bool)` — `skipImages` has no boolean prefix. Should be `shouldSkipImages`. | Low |

---

## `blogs/builder/services/asset/discovery.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 57 | ~55 | **§3.4 Boolean Naming** | `syncStaticAssets(ctx, bgCtx context.Context, skipImages bool)` — `skipImages` should be `shouldSkipImages`. | Low |
| 58 | ~95 | **§3.4 Boolean Naming** | Local `sameStatic` should be `isSameStaticDir`. | Low |
| 59 | ~58 | **§3.4 Boolean Naming** | Local `debugAssets` should be `isDebugEnabled` or `debugAssetsEnabled`. | Low |
| 60 | ~55–120 | **§4.5 Function Length** | `syncStaticAssets` is approximately 65 lines. | Medium |
| 61 | ~130–210 | **§4.5 Function Length** | `setupImageEnqueue` is approximately 80 lines. | Medium |

---

## `blogs/builder/services/cache/interfaces.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 62 | ~13–35 | **§7.1 Interface Size** | `Service` embeds four interfaces (`PostCache`, `SearchCache`, `SocialCardCache`, `BuildArtifactCache`) and adds its own 9 methods. The combined method count far exceeds the 6-method flag threshold. | Medium |
| 63 | — | **§7.4 Interface Assertions** | No `var _ Service = (*cacheService)(nil)` in the `cache` service package. | Low |

---

## `blogs/builder/services/post/service.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 64 | — | **§7.4 Interface Assertions** | No `var _ Service = (*postService)(nil)` in the `post` package. | Low |

---

## `blogs/builder/services/post/interfaces.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 65 | ~51 | **§3.1 Variable Naming** | `ReconfigureWithReporter(r ui.Reporter, l *slog.Logger)` — `l` is prohibited for a logger. | Low |
| 66 | ~48 | **§3.1 Variable Naming** | `SetAssetsGate(ch <-chan struct{})` — `ch` is prohibited for a channel. | Low |
| 67 | ~47–57 | **§7.1 Interface Size** | `Service` has **8 methods**. Exceeds the 6-method flag threshold. | Medium |

---

## `blogs/builder/services/post/process.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 68 | ~70 | **§8.2 Mutex Discipline** | `renderTasksMu sync.Mutex` field inside `renderTaskCollector` struct has no comment explaining what shared state it protects. | Medium |
| 69 | ~100–200 | **§4.5 Function Length** | `ProcessStreaming()` is approximately 100 lines — nearly double the hard limit. | High |
| 70 | ~240–300 | **§4.5 Function Length** | `runStreamingRenderPhase()` is approximately 60 lines — at/over the limit. | Medium |

---

## `blogs/builder/services/post/worker.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 71 | ~55–135 | **§4.5 Function Length** | `aggregateLocal()` is approximately 80 lines. | Medium |

---

## `blogs/builder/services/post/post_parser.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 72 | 1–30 | **§2.3 Import Grouping** | **4 import groups** instead of 3 and in the wrong order: stdlib → internal (cache, config, etc.) → more internal (parser, renderer) → third-party (goldmark). Third-party packages must appear as group 2, before all internal packages. | Medium |
| 73 | ~67–72 | **§9.1 Context Always First** | `parseMarkdownWithRecovery(bodyOnly []byte, path string, mdPool *sync.Pool, ctx context.Context)` — `ctx context.Context` is the **last** parameter instead of the first. | High |
| 74 | ~175–185 | **§6.1 Error Wrapping** | `ParseMarkdown` returns `return nil, err` for both `ParseMarkdownMetadata` errors and `RenderParsedMarkdown` errors without adding any wrapping context (e.g. `fmt.Errorf("parsing %s: %w", opts.Path, err)`). | Medium |

---

## `blogs/builder/services/post/post_single.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 75 | 1–15 | **§2.3 Import Grouping** | **4 groups**: stdlib → `afero` (third-party) → most internal packages → `utils/timeutil` isolated in a 4th group. `timeutil` should be in the same internal group. | Medium |
| 76 | ~95 | **§6.1 Error Wrapping** | `return err` for the `s.sourceFs.Stat(path)` error is not wrapped. Should be `fmt.Errorf("stat %s: %w", path, err)`. | Medium |
| 77 | ~100 | **§6.1 Error Wrapping** | `return err` for the `afero.ReadFile(s.sourceFs, path)` error is not wrapped. Should be `fmt.Errorf("read %s: %w", path, err)`. | Medium |
| 78 | ~65–140 | **§4.5 Function Length** | `ProcessSingleWithResult()` is approximately 75 lines. | Medium |

---

## `blogs/builder/services/post/post_math.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 79 | ~17 | **§3.1 Variable Naming** | In `processCachedMath`: `if s, ok := v.(string); ok { renderedMath[expr.Hash] = s }` — the short variable `s` is used for a string, shadowing the method receiver `s *postService`. `s` is explicitly prohibited as a name for strings. | Medium |
| 80 | ~45 | **§3.1 Variable Naming** | In `renderMath`: same `if s, ok := v.(string); ok` pattern — `s` used for string, shadowing receiver `s`. | Medium |

---

## `blogs/builder/services/post/parser_helpers.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 81 | 1–30 | **§2.3 Import Grouping** | **4 groups** in wrong order: stdlib → internal (hashing, models, etc.) → third-party (goldmark packages) → more internal (mdParser). Must be: stdlib → third-party → internal, in 3 groups. | Medium |

---

## `blogs/builder/services/render/render_service.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 82 | 1–15 | **§2.3 Import Grouping** | Import groups are in **wrong order**: stdlib → internal → third-party (`afero`). The required order is stdlib → third-party → internal. | Medium |
| 83 | — | **§7.4 Interface Assertions** | No `var _ Service = (*renderService)(nil)` in the `render` package. | Low |

---

## `blogs/builder/services/render/interfaces.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 84 | ~13–30 | **§7.1 Interface Size** | `Service` interface has **14 methods** (`ReconfigureForBuild`, `SetAssetsGate`, `ReconfigureWithLogger`, `RenderPage`, `RenderIndex`, `Render404`, `RenderGraph`, `RegisterFile`, `SetAssets`, `GetAssets`, `GetRenderedFiles`, `ClearRenderedFiles`, `ReloadTemplates`, `Has404Template`). This is a severe violation of the 1–3 ideal / 6+ flag threshold. | High |
| 85 | ~14 | **§3.1 Variable Naming** | `ReconfigureWithLogger(l *slog.Logger)` — `l` is prohibited for a logger name. | Low |
| 86 | ~13 | **§3.1 Variable Naming** | `SetAssetsGate(ch <-chan struct{})` — `ch` is prohibited for a channel name. | Low |

---

## `blogs/builder/services/scanner/scanner.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 87 | ~50 | **§18.3 Logger Injection** | `slog.Default()` is passed directly to `async.FireAndForget` in `ScanStreaming`. The `metadataScanner` struct has no logger field; it relies entirely on the global default logger. | Medium |
| 88 | ~90–98 | **§6.1 Error Wrapping** | In `ScanStreaming`, the `afero.Walk` error and the `errgroup.Wait()` error are returned raw (`errChan <- err`, `return err`) with no wrapping context. | Medium |
| 89 | — | **§7.4 Interface Assertions** | No `var _ Scanner = (*metadataScanner)(nil)`. | Low |

---

## `blogs/builder/services/wasm/wasm_service.go`

| # | Line | Rule | Violation | Severity |
|---|------|------|-----------|----------|
| 90 | 1–20 | **§2.3 Import Grouping** | `"github.com/spf13/afero"` (third-party) is placed in the same import group as the internal packages. Only 2 groups when 3 are needed (stdlib → third-party → internal). | Medium |
| 91 | — | **§7.4 Interface Assertions** | No `var _ Service = (*wasmService)(nil)`. | Low |

---

## Cross-Cutting Violations (All Packages)

| # | Location | Rule | Violation | Severity |
|---|----------|------|-----------|----------|
| 92 | All service packages | **§7.4 Interface Assertions** | No `var _ Interface = (*ConcreteType)(nil)` compile-time assertion exists anywhere across all 6 service packages (`asset`, `cache`, `post`, `render`, `scanner`, `wasm`) and the `consoleHandler` / `mockPostService` types. Every major implementation is missing this guard. | Low |
| 93 | `orchestration/logger.go`, `services/post/post_service_mock.go` | **§15.1 One Concept Per File** | `logger.go` conflates a concrete `consoleHandler` slog handler type with unrelated global `DevLog*` utility functions and `HTTPLog`. These are distinct concepts and should be in separate files. | Low |
| 94 | `services/post/interfaces.go`, `services/asset/interfaces.go`, `services/render/interfaces.go` | **§9.2 Context in Struct** | `SiteWideOptions.Ctx context.Context`, `ProcessPostsOptions.Ctx context.Context`, `ScanOptions.Ctx context.Context`, `WorkerContext.Ctx context.Context` — `context.Context` is stored in short-lived options structs without documentation that the struct lifetime equals the context lifetime. Per §9.2, this requires explicit documentation even when arguably acceptable. | Low |

---

## Summary by Severity

| Severity | Count |
|----------|-------|
| **High** | 13 |
| **Medium** | 47 |
| **Low** | 34 |
| **Total** | **94** |

### Top-Priority Fixes (High Severity)

1. **§4.2** — `handleImageTask`, `handleNonImageTask`, `startImageWorkers`, `startNonImageWorkers` in `pipeline.go` all exceed the 4-parameter limit; refactor into an options struct.
2. **§6.3** — `CopyDirVFS` collects a slice of errors but returns only `errs[0]`; switch to `errors.Join`.
3. **§14.3** — The entire `CheckWASM*`/`DeployWASM*` family returns `bool`; these I/O operations must return `error`.
4. **§4.5** — `ProcessStreaming` (~100 lines) needs to be decomposed into smaller units.
5. **§14.4 / §8.6** — The `imageCacheWriter` goroutine is process-immortal with no shutdown or context cancellation mechanism.
6. **§9.1** — `parseMarkdownWithRecovery` has `ctx context.Context` as its last parameter; it must be first.
7. **§7.1** — `render.Service` (14 methods) is dramatically over-sized and must be decomposed.