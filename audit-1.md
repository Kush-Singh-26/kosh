Style-Guide Audit Report

**Packages audited:** `builder/cache/` (all subdirs), `builder/models/`, `builder/context/`, `builder/config/`

---

## §2.1 — Package Naming

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 1 | `builder/context/context.go`, `builder/context/doc.go`, `builder/context/errors.go` | 1 | Package declared as `buildCtx` — mixed-case, not a single lowercase word. Must be `buildctx` (or similar). All three files in the `context/` directory are affected. | **High** |

---

## §2.3 — Import Grouping

The rule mandates **exactly three blank-line-separated groups**: (1) stdlib, (2) third-party, (3) internal. Never mix groups.

| # | File | Lines | Violation | Severity |
|---|------|-------|-----------|----------|
| 2 | `builder/cache/cache.go` | 3–20 | Only 2 groups. Internal packages (`builder/cache/core`, `gc`, `migrate`, `store`) and third-party (`hashicorp/golang-lru`, `go.etcd.io/bbolt`) are fused into a single second group. | Medium |
| 3 | `builder/cache/cache_queries.go` | 3–12 | Three groups but **wrong order**: stdlib → internal → third-party. Correct order is stdlib → third-party → internal. | Medium |
| 4 | `builder/cache/cache_reads.go` | 3–14 | Three groups but mixed: group 2 is internal-only (`core`), group 3 mixes internal (`fspkg`) with third-party (`bbolt`, `errgroup`). | Medium |
| 5 | `builder/cache/cache_writes.go` | 3–23 | **Four** groups: stdlib → internal(`core`,`gc`) → third-party(`bbolt`,`errgroup`) → internal(`fspkg`,`models`). Third-party wedged between two internal groups. | Medium |
| 6 | `builder/cache/gc/gc_run.go` | 3–12 | Two groups: stdlib then a single block mixing internal (`cache/core`, `cache/store`) with third-party (`go.etcd.io/bbolt`). | Medium |
| 7 | `builder/cache/gc/verify.go` | 3–11 | Two groups: stdlib then a block mixing internal (`cache/core`, `cache/store`, `builder/fs`) with third-party (`bbolt`). | Medium |
| 8 | `builder/cache/gc/refcount.go` | 3–9 | Two groups: stdlib then a block mixing internal (`cache/core`) with third-party (`bbolt`). | Medium |
| 9 | `builder/cache/store/store.go` | 3–22 | Three groups but **wrong order**: stdlib → internal → third-party. | Medium |
| 10 | `builder/cache/migrate/migrations.go` | 3–13 | Two groups: stdlib then a block mixing internal (`cache/core`) with third-party (`bbolt`, `bbolterrors`). | Medium |
| 11 | `builder/config/config.go` | 3–15 | **Four** groups: stdlib → mixed(internal `builder/fs`, `builder/models` + third-party `afero`) → third-party(`yaml.v3`). | Medium |

---

## §2.4 — Avoid Package-Level Mutable State

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 12 | `builder/cache/types_aliases.go` | 37–53 | Six mutable `var` package-level re-export aliases: `DefaultGCConfig`, `HashContent`, `HashString`, `GeneratePostID`, `Encode`, `Decode`. These are function-valued variables and can be reassigned. Only `ErrXxx` vars and `sync.Once` caches are permitted. | Medium |
| 13 | `builder/cache/store/store.go` | 15–21 | `var level3EncoderPool = sync.Pool{…}` — a `sync.Pool` is not a `sync.Once`; it is mutable shared state. | Medium |
| 14 | `builder/cache/store/store.go` | ~54 | `var storeTempCounter atomic.Uint64` — mutable global counter. | Medium |
| 15 | `builder/cache/store/store.go` | ~155 | `var dirMutexes [dirMutexBuckets]sync.Mutex` — mutable global array of mutexes. | Medium |
| 16 | `builder/cache/gc/gc_run.go` | ~16 | `var ssrTypes = []string{"d2", "math", …}` — mutable global slice (elements could be appended/mutated). Should be a typed const array or package-level function returning the slice. | Medium |
| 17 | `builder/cache/migrate/migrations.go` | ~36 | `var registeredMigrations = []Migration{…}` — mutable global slice. Should be returned by a function or declared as a file-level `const`-like construct. | Medium |
| 18 | `builder/models/constants.go` | 6–21 | `var AlwaysSyncPaths = map[string]bool{…}` — mutable global map exported by name; callers can add or delete entries. Should be a function `func AlwaysSyncPaths() map[string]bool` or a frozen `mapset`. | Medium |

---

## §3.1 — Variable Naming

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 19 | `builder/cache/cache_reads.go` | ~203 | `c := bucket.Cursor()` inside `GetPostsByTemplate` — single-letter variable `c` used for a cursor. Only `i`, `j`, `k`, `err`, and receiver names are permitted as single-letter variables. Rename to `cur` or `cursor`. | Low |
| 20 | `builder/cache/gc/verify.go` | ~25, ~60 | `for k, v := cursor.First()` and in `ssrBucket.ForEach(func(k, v []byte)…)` — `v` is a single-letter variable that is not a permitted loop index. Rename to `val` or `data`. | Low |
| 21 | `builder/cache/gc/gc_run.go` | ~53, ~75 | Lambda parameters `func(_, v []byte)` in `postsBucket.ForEach` and `ssrBucket.ForEach` — `v` is a non-permitted single-letter variable. | Low |
| 22 | `builder/cache/gc/refcount.go` | ~86, ~98 | `refBucket.ForEach(func(k, _ []byte)…)` and `postsBucket.ForEach(func(_, v []byte)…)` — `v` as value variable violates the single-letter restriction. | Low |

---

## §3.4 — Boolean Naming

Boolean fields and functions returning `bool` must read as a question using `Is`, `Has`, `Can`, or `Should` prefix.

| # | File | Lines | Violation | Severity |
|---|------|-------|-----------|----------|
| 23 | `builder/config/config.go` | ~63, ~115–117 | `BuildOptions.CompressImages bool`, `BuildOptions.MinifySVGs bool` — neither starts with a boolean-question prefix. | Low |
| 24 | `builder/config/config.go` | ~122–125 | `Config.ForceRebuild bool`, `Config.ForceLock bool`, `Config.IncludeDrafts bool` — none start with `Is`/`Has`/`Should`. | Low |
| 25 | `builder/models/models.go` | ~20, ~22, ~47–48 | `LightPostMetadata.Pinned bool`, `LightPostMetadata.Draft bool`, `ScannedFile.Pinned`, `ScannedFile.Draft`, `PostMetadata.Pinned`, `PostMetadata.Draft` — should be `IsPinned`, `IsDraft`. | Low |
| 26 | `builder/models/models.go` | ~175–176 | `GeneratorsConfig.Sitemap bool`, `.RSS bool`, `.PWA bool`, `.Search bool` — none start with a boolean-question prefix. `FeaturesConfig.RawMarkdown bool` — same. `GraphConfig.Enabled bool`, `GraphConfig.ShowTags bool` — same. | Low |
| 27 | `builder/models/cache.go` | ~18, ~20, ~63 | `PostMeta.Pinned bool`, `PostMeta.Draft bool` — should be `IsPinned`, `IsDraft`. `SSRArtifact.Compressed bool` — should be `IsCompressed`. | Low |
| 28 | `builder/models/ssr.go` | ~52 | `MathExpression.DisplayMode bool` — should be `IsDisplayMode` or `IsBlock`. | Low |
| 29 | `builder/cache/gc/config.go` | ~14, ~25 | `GCConfig.DryRun bool` — should be `IsDryRun`. `GCResult.WasSkipped bool` — "Was" is not in the permitted prefix set; should be `IsSkipped`. | Low |
| 30 | `builder/cache/store/store.go` | ~102 | `func fileExists(path string) bool` — function returning bool does not start with `Is`/`Has`/`Can`/`Should`. Should be `isFilePresent` or `hasFile`. | Low |

---

## §4.3 — Return Values

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 31 | `builder/models/seo.go` | ~26–34 | `GeneratePostJSONLD` returns only `template.HTML` and silently swallows `json.Marshal` errors (returns `""` on error). The function signature should be `(template.HTML, error)` so callers can react. Per §4.3 the error must be propagated, not buried. | High |

---

## §4.5 — Function Length (~50 lines max, hard limit ~60)

| # | File | Lines | Violation | Severity |
|---|------|-------|-----------|----------|
| 32 | `builder/cache/store/store.go` | `Put` function ~165–260 | `Put` is ~90+ lines: it manages two different encoder code paths (pool-based Level-3 and fast encoder), temp file creation, write, close, race-safe rename, and all error branches. Far exceeds the 50-line target. | Medium |
| 33 | `builder/cache/cache_writes.go` | `encodePosts` ~33–110 | ~80 lines; parallel encode with error accumulation for post data, search records, and deps. | Medium |
| 34 | `builder/cache/cache_writes.go` | `BatchStoreSSR` ~310–415 | ~90 lines; parallel file I/O + BoltDB batch transaction. | Medium |
| 35 | `builder/cache/cache_writes.go` | `DeletePost` ~420–490 | ~65 lines; large `db.Update` transaction with nested tag deletion loops. | Medium |
| 36 | `builder/cache/gc/refcount.go` | `ReconcileWithLog` ~85–155 | ~70 lines; full refcount clear-and-recompute with discrepancy logging. | Medium |
| 37 | `builder/config/build_config.go` | `validate` ~150–230 | ~75 lines of if/clamp blocks for ~11 independent config fields. | Medium |
| 38 | `builder/cache/cache_reads.go` | `GetPostsByIDs` ~115–185 | ~60 lines; raw fetch + parallel-or-sequential decode branch. Borderline. | Low |

---

## §6.4 — Explicit Error Ignoring (must use `_ = expr` **with a comment**)

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 39 | `builder/cache/maintenance.go` | 8–9 | `_ = m.db.Close()` and `_ = os.RemoveAll(m.basePath)` — both lack any explanatory comment. | Medium |
| 40 | `builder/cache/cache.go` | `clearFilesystemStore` ~305 | `_ = m.store.Delete(category, hash)` inside loop — no comment explaining why the error is discarded. | Medium |
| 41 | `builder/cache/gc/gc_run.go` | `reconcileHTMLRefCounts` ~78 | `_ = refCount.ReconcileWithLog(slog.Default())` — return value silently discarded, no comment. | Medium |
| 42 | `builder/cache/gc/gc_run.go` | `reconcileSSRRefCounts` ~82–105 | Four separate `_ =` discards (`db.Update`, `postsBucket.ForEach`, `s.Delete`, `ssrBucket.Put`) — none have explanatory comments. | Medium |
| 43 | `builder/cache/gc/gc_run.go` | `updateGCStats` ~108–120 | `_ = db.Update(…)` and two `_ = statsBucket.Put(…)` — no comments. | Medium |
| 44 | `builder/cache/gc/refcount.go` | `Get` ~60 | `_ = m.db.View(…)` — error silently discarded, no comment. `Get` should return `(uint32, error)` to properly propagate view failures. | Medium |
| 45 | `builder/cache/gc/refcount.go` | `ReconcileWithLog` ~88–101 | `_ = refBucket.ForEach(…)`, `_ = postsBucket.ForEach(…)`, and `_ = refBucket.Put(…)` — three uncommented discards. | Medium |
| 46 | `builder/cache/cache_reads.go` | `GetPostsByIDs` ~180 | `_ = g.Wait()` — errgroup wait result discarded without comment. Any decode goroutine that returns an error would be silently lost. | High |
| 47 | `builder/cache/cache_writes.go` | `BatchStoreSSR` ~318 | `g, _ := errgroup.WithContext(context.Background())` — the derived cancel-context is discarded via `_` without comment, meaning goroutine cancellation is impossible. | High |
| 48 | `builder/cache/store/store.go` | `Put` ~225–245 | Multiple `_ = f.Close()` and `_ = os.Remove(tmpPath)` in error-path cleanup branches — present but lack the required comment explaining why the error is acceptable to ignore. | Low |

---

## §7.1 — Interface Size (6+ methods is a flag)

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 49 | `builder/models/interfaces.go` | ~13–22 | `RenderService` has **12 methods** — well over the 6-method flag threshold. Should be split (e.g., separate `TemplateRenderer`, `AssetRegistry`, `LifecycleController`). | Medium |
| 50 | `builder/models/interfaces.go` | ~24–32 | `ArtifactSink` has **8 methods** — exceeds threshold. | Medium |
| 51 | `builder/models/interfaces.go` | ~44–55 | `BuildArtifactCache` has **8 methods** — exceeds threshold. | Medium |
| 52 | `builder/models/interfaces.go` | ~34–41 | `PostCache` has **6 methods** — at the threshold. | Low |
| 53 | `builder/models/models.go` | `TemplateConfig` interface | `TemplateConfig` has **6 methods** — at the flag threshold. | Low |

---

## §7.4 — Interface Assertions

`var _ InterfaceName = (*Type)(nil)` must appear at package level for every major implementation.

| # | File | Violation | Severity |
|---|------|-----------|----------|
| 54 | `builder/cache/cache.go` | `*Manager` implicitly satisfies `models.PostCache`, `models.SearchCache`, `models.SocialCardCache`, and `models.BuildArtifactCache`, but no compile-time assertion exists for any of them. A missing `ClearDirty()` method on `Manager` means it likely does **not** actually satisfy `BuildArtifactCache` — without an assertion, this bug is invisible. | **High** |
| 55 | `builder/config/config.go` | `*Config` implements `models.TemplateConfig` (six methods present) but no `var _ models.TemplateConfig = (*Config)(nil)` assertion. | Medium |
| 56 | `builder/cache/adapter.go` | `*DiagramCacheAdapter` has `Load`/`Store` methods intended to satisfy an `SSRMap`-like interface, but there is no assertion anywhere. | Medium |

---

## §8.2 — Mutex Discipline

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 57 | `builder/cache/store/store.go` | ~155 | `var dirMutexes [dirMutexBuckets]sync.Mutex` has **no comment** explaining what each bucket protects. §8.2 requires every mutex to carry such a comment. | Medium |
| 58 | `builder/cache/cache.go` | `VerifyCacheID` and `SetCacheID` methods | Both methods write to `m.cacheID` **without holding `m.mu`**, even though the mutex comment explicitly states it protects `dirty and cacheID`. This is a data-race on `cacheID`. | **High** |
| 59 | `builder/cache/store/store.go` | `ensureDir` ~147–149 | **Double-unlock bug**: `defer mu.Unlock()` is registered at function entry (line ~124), then the error path inside the `for i` loop calls `mu.Unlock()` explicitly before `return err`. When that `return` is executed the deferred unlock fires a second time → **panic: sync: unlock of unlocked mutex**. This is a correctness bug, not just style. | **High** |

---

## §8.6 — Context Propagation / §9.1 — Context Always First

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 60 | `builder/cache/store/store.go` | `ListHashes`, `Size`, `CleanOrphans` | None of these functions accept a `ctx context.Context` as their first parameter (§9.1 violation), yet they spawn `fspkg.ParallelWalk` goroutines with a hard-coded `context.Background()`. Callers cannot cancel mid-operation. | Medium |
| 61 | `builder/cache/cache_writes.go` | `BatchStoreSSR` ~318 | `errgroup.WithContext(context.Background())` creates a non-cancellable context. The function should accept `ctx context.Context` as its first param and thread it through. | Medium |
| 62 | `builder/cache/cache_reads.go` | `GetPostsByIDs` ~155–180 | Goroutines launched via `g.Go` perform parallel decodes with no `ctx.Done()` check; long-running decode storms cannot be interrupted. | Low |

---

## §13.1 — Package Doc Comment

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 63 | `builder/cache/core/types.go` | 1 | Doc comment reads `// Package cache provides…` but the package name is **`core`**, not `cache`. The wrong package name makes `go doc` output misleading. | Medium |
| 64 | `builder/models/ssr.go` | 1–4 | A second package-level doc comment exists here (`// Package models provides shared data structures for SSR…`), while `models.go` already has the authoritative one. Two competing package doc comments in the same package create confusing `go doc` output. The package has no `doc.go`; the doc should be consolidated there. | Low |

---

## §13.2 — Exported Identifiers Must Have Doc Comments

| # | File | Identifier | Severity |
|---|------|------------|----------|
| 65 | `builder/cache/cache_batch_helpers.go` | `EncodedPost` struct | No doc comment. | Low |
| 66 | `builder/cache/maintenance.go` | `Clear()`, `Rebuild()` | Neither exported method has a doc comment. | Low |
| 67 | `builder/cache/core/schema.go` | `AllBuckets()` function | No doc comment. | Low |
| 68 | `builder/cache/gc/config.go` | `GCConfig` struct, `GCResult` struct | Neither type has a doc comment. | Low |
| 69 | `builder/cache/migrate/migrations.go` | `Migration` struct | No doc comment. | Low |
| 70 | `builder/cache/store/store.go` | `Store` struct | No doc comment on the type itself. | Low |
| 71 | `builder/models/constants.go` | `AlwaysSyncPaths` var | No doc comment. | Low |
| 72 | `builder/models/ssr.go` | `(SSRArtifactType).String()`, `ParseSSRType()` | Neither exported function has a doc comment. | Low |

---

## §14.1 — Magic Numbers / Magic Strings

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 73 | `builder/cache/cache.go` | `IncrementBuildCount` ~370 | String literal `"builds_since_gc"` used directly instead of a named constant. This string also appears in `gc_run.go`; any future rename is a multi-file grep hunt. | Medium |
| 74 | `builder/cache/gc/gc_run.go` | `updateGCStats` ~110 | Same `"builds_since_gc"` string literal repeated. Should reuse the same constant defined in `core`. | Medium |
| 75 | `builder/models/constants.go` | `GetDefaultWorkerCount` ~43 | Magic number `2` used as the minimum worker floor. Should be a named constant `minWorkerCount = 2`. | Low |

---

## §14.3 — Bool Return (use `error` when more informative)

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 76 | `builder/cache/cache.go` | `VerifyCacheID` ~255 | Returns `(bool, error)` but the `bool` semantics are inverted (`true` = IDs **don't** match, `false` = they do), making the return confusing. The function also has the side-effect of mutating `m.cacheID` without the mutex (see §8.2 above). The bool would be far clearer as a dedicated sentinel error or a separate `NeedsIDUpdate() bool` function. | Medium |

---

## §14.7 — `any`/`interface{}` Overuse

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 77 | `builder/cache/adapter.go` | `local map[string]any`, `dirty map[string]any` ~15–16 | Values are documented as `string`, `[]byte`, or `models.SSRThemePair`. A small typed discriminated union interface (e.g., `SSRValue`) would eliminate unchecked type assertions throughout `Get`, `Set`, `Flush`, and `Merge`. | Low |

---

## §15.1 — One Concept Per File

| # | File | Violation | Severity |
|---|------|-----------|----------|
| 78 | `builder/models/models.go` | The file contains scanner types (`LightPostMetadata`, `ScannedFile`, `ScannedAsset`), page-render types (`PageData`, `Paginator`, `Breadcrumb`), config types (`MenuEntry`, `AuthorConfig`, `FeaturesConfig`, `TemplateConfig` interface), sitemap/RSS structs, graph structs, **and** the entire search index model (`PostRecord`, `IndexedPost`, `SearchIndex`) plus four encoding helper functions. This is at least six distinct concepts in one file. | Medium |

---

## §15.2 — File Size (500-line hard limit)

| # | File | Line Count | Violation | Severity |
|---|------|-----------|-----------|----------|
| 79 | `builder/models/models_gen.go` | ~2,130 lines | Generated, but still ~4× the hard limit. The generator directive should split output by type (one generated file per source type). | Low |
| 80 | `builder/cache/cache_writes.go` | ~510 lines | Marginally over the hard limit; dominated by two large functions (`encodePosts`, `BatchStoreSSR`) that should be extracted. | Low |

---

## §18.3 — Logger Injection (no `slog.Default()` mid-logic)

| # | File | Line | Violation | Severity |
|---|------|------|-----------|----------|
| 81 | `builder/cache/cache.go` | `openStoreWithCleanup`, `newMemCacheWithCleanup`, `OpenWithTimeout`, `ClearAll` | Direct `slog.Error` / `slog.Warn` calls throughout the `Open*` and lifecycle methods. `Manager` should accept a `*slog.Logger` at construction time (already has one in `BuildContext`). | Medium |
| 82 | `builder/cache/cache_writes.go` | `encodePosts` ~48, `logBatchCommitFailure` ~222 | `slog.Warn(…)` and `slog.Error(…)` called directly instead of through an injected logger. | Medium |
| 83 | `builder/cache/adapter.go` | `Flush` ~115 | `slog.Info("Flushing diagram cache…")` — should use an injected logger. | Low |
| 84 | `builder/cache/gc/gc_run.go` | `reconcileHTMLRefCounts` ~78 | `slog.Default()` passed directly as a logger argument — the caller (`RunGC`) receives no logger parameter at all; the logger should be threaded through. | Medium |
| 85 | `builder/cache/migrate/migrations.go` | `RunMigrations` ~155 | `if logger == nil { logger = slog.Default() }` — falls back to the global default instead of requiring the caller to provide a logger. | Medium |
| 86 | `builder/config/config.go` | `loadConfigFile` ~155–165 | `slog.Warn(…)` called directly; `LoadFs` / `Load` do not accept a logger. Config loading errors should be surfaced to the injected logger of the calling component. | Medium |

---

## Summary Table by Severity

| Severity | Count |
|----------|-------|
| **High** | 8 (items 1, 31, 46, 47, 54, 58, 59) |
| **Medium** | 54 |
| **Low** | 24 |
| **Total** | **86 violations** |

---

## Top Prioritised Fixes

1. **#59 (§8.2) — Double-unlock panic in `store/store.go` `ensureDir`**: The `defer mu.Unlock()` + explicit `mu.Unlock()` before `return err` in the mkdir loop will panic at runtime. Remove the manual `mu.Unlock()` call; the `defer` handles it.

2. **#58 (§8.2) — Data race on `Manager.cacheID`**: `VerifyCacheID` and `SetCacheID` both write `m.cacheID` without holding `m.mu`, despite the mutex comment explicitly naming `cacheID` as a protected field.

3. **#54 (§7.4) — Missing interface assertions for `Manager`**: Without `var _ models.BuildArtifactCache = (*Manager)(nil)`, a missing `ClearDirty()` method (which appears nowhere in the audited files) silently leaves `Manager` out of conformance — bugs that would only surface at runtime when an assignment is attempted.

4. **#31 (§4.3) — `GeneratePostJSONLD` swallows errors**: Returns empty HTML with no indication of failure; callers cannot distinguish "no JSON-LD" from "serialisation failure."

5. **#46 & #47 (§6.4) — `_ = g.Wait()` and `g, _ := errgroup.WithContext(…)`**: Decode errors in `GetPostsByIDs` are silently dropped; and `BatchStoreSSR` creates an errgroup with an uncancellable `context.Background()`.

6. **#1 (§2.1) — Package `buildCtx` mixed-case name**: Violates the most fundamental Go naming rule; must be `buildctx`.