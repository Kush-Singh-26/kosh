# Kosh Implementation Plan - Road to 95.0

**Current Status:** 84.4/100 strict (target: 95.0, need +10.6)
**Last Updated:** 2026-03-15 (Subjective Review Blocked - Technical Limitations)
**Commits:** 37 ahead of origin/main
**Commit Range:** 64251f4 → 07e9cab (Phase 2 + Tiers 1-2 + Integration Tests)

---

## Executive Summary

### Score Progress (Current Scan)
| Dimension | Baseline | Current | Change | Priority |
|-----------|----------|---------|--------|----------|
| **High-level elegance** | 80.0% | 88.5% | +8.5 | ✅ Done |
| **Design coherence** | 72.5% | 82.5% | +10.0 | ✅ Done |
| **Cross-module arch** | 68.5% | 72.5% | +4.0 | ✅ Phase 1 Done |
| **Mid-level elegance** | 78.5% | 78.5% | 0 | ✅ Phase 2 Done |
| **Test strategy** | 68.5% | 68.5% | 0 | ⏳ Blocked (Subjective) |
| **AI generated debt** | 71.0% | 71.0% | 0 | ⏳ Blocked (Subjective) |

### Score Analysis - Biggest Drags
| Dimension | Score | Gap | Weight | Impact |
|-----------|-------|-----|--------|--------|
| **Test health (strict)** | 48.9% | -46.1% | High | 🔴 Biggest breakthrough opportunity |
| **Mid elegance** | 78.5% | -21.5% | 17.9% | 🔴 -2.88 pts drag |
| **High elegance** | 88.5% | -11.5% | 17.9% | 🟡 -1.54 pts drag |
| **Test strategy** | 68.5% | -26.5% | High | 🔴 Subjective drag |
| **Contracts** | 82.0% | -18.0% | 9.8% | 🟡 -1.32 pts drag |

### Key Insight
**Score is subjective-review locked at 84.4.** Mechanical improvements (tests, architectural fixes) are complete, but subjective dimensions require human/AI review to recognize improvements.

**Path to 95.0:** Complete 20 subjective review batches (currently blocked by technical limitations with codex runner - prompts exceed Windows command line length limits).

---

## Completed Improvements

### Phase 1: Architectural Decoupling ✅
- ✅ Split Builder into `BuilderDependencies` + `BuilderState`
- ✅ Consolidated service interfaces with `ReconfigureForBuild()`
- ✅ Channel-based synchronization replacing callbacks
- ✅ Extensive documentation for integration seams
- ✅ WASM decoupling from `internal/build`
- ✅ **Cache→Renderer decoupling verified** - Already complete (uses `builder/models`)

### Phase 2: Mid-Level Elegance ✅
- ✅ Phase 2.1: Removed dual sync channels (`metadataReadyCh` consolidated)
- ✅ Phase 2.2: Standardized fire-and-forget with `FireAndForgetWithCleanup`
- ✅ Phase 2.3: Verified `BatchCommit` already centralized
- ✅ Phase 2.4: Added `DiskSink.WriteStream` panic safety with defer/recover

### Phase 3: Test Coverage ✅ COMPLETE
- ✅ **Tier 1: Quick Wins** - 27 tests for utils (53.5% coverage)
- ✅ **Tier 2: Search Tests** - 40+ tests for search (80.1% coverage)
- ✅ **Tier 2.5: Integration Tests** - 10 incremental build tests (builder/run)
- ✅ **Test Fix:** Fixed `internal/clean/clean_test.go` compilation error

### Test Summary
| Package | Tests Added | Coverage | Status |
|---------|-------------|----------|--------|
| `builder/utils` | 27 | 53.5% | ✅ Complete |
| `builder/search` | 40+ | 80.1% | ✅ Complete |
| `builder/run` | 10 | - | ✅ Complete |
| `internal/clean` | Fix | - | ✅ Complete |
| **Total** | **77+ tests** | **Varies** | ✅ **28/28 run tests pass** |

---

## Test Coverage Progress

### Completed Tests (67+ total)

#### Tier 1: Utils Package (27 tests, 53.5% coverage)
| File | Tests | Coverage | Status |
|------|-------|----------|--------|
| `async_test.go` | 13 | 53.5% | ✅ Complete |
| `retry_test.go` | 14 | 53.5% | ✅ Complete |

**Bug fixes discovered:**
- `FireAndForgetWithCleanup`: defer order bug (cleanup now runs on panic)
- `maxRetries=0`: documented behavior (returns nil without attempting)

#### Tier 2: Search Package (40+ tests, 80.1% coverage)
| File | Tests | Coverage | Status |
|------|-------|----------|--------|
| `stemmer_test.go` | 5 | 80.1% | ✅ Complete |
| `fuzzy_test.go` | 35+ | 80.1% | ✅ Complete |

**Test breakdown:**
- Stemmer: 57 word cases, cache behavior, edge cases, consistency
- Fuzzy: Levenshtein distance, fuzzy match, trigrams, query parsing

### Remaining Untested Modules (9 issues)
| File | LOC | Priority | Status |
|------|-----|----------|--------|
| `builder/assets/wasm.go` | 227 | T3 | ⏳ Pending |
| `builder/parser/unified.go` | 411 | T3 | ⏳ Pending |
| `builder/search/fuzzy.go` | 365 | T2 | ✅ Tested |
| `builder/search/stemmer.go` | 371 | T2 | ✅ Tested |
| `builder/utils/async.go` | 65 | T3 | ✅ Tested |
| `builder/utils/retry.go` | 82 | T3 | ✅ Tested |
| `cmd/kosh/cache_cmd.go` | 239 | T3 | ⏳ Pending |
| `cmd/kosh/ui.go` | 185 | T3 | ⏳ Pending |

---

## Remaining Work

### Phase 1: Architectural Decoupling (High Impact, ~+4.0 points)

#### 1.1 Fix Cache→Renderer Upward Dependency [T1-High]
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
```

**Estimated Effort:** 2-3 hours  
**Risk:** Medium - requires updating all SSR cache call sites  
**Testing:** Verify D2/LaTeX SSR cache hit/miss behavior unchanged

---

### Phase 2: Test Coverage Completion (~+4-6 points)

#### 2.1 Test WASM Deployment [T3]
**File:** `builder/assets/wasm.go` (227 LOC)  
**Tests to add:**
```go
// wasm_test.go
func TestCheckWASM_DeploysEmbedded(t *testing.T)
func TestCompileWASMFromSource_Success(t *testing.T)
func TestDeployWASMFromFile(t *testing.T)
```

**Effort:** 2-3 hours  
**Impact:** Removes 1 untested module issue

#### 2.2 Test Markdown Parser [T3]
**File:** `builder/parser/unified.go` (411 LOC)  
**Tests to add:**
```go
// unified_test.go
func TestUnifiedParser_FrontmatterExtraction(t *testing.T)
func TestUnifiedParser_BodyParsing(t *testing.T)
func TestUnifiedParser_MathExpressions(t *testing.T)
```

**Effort:** 3-4 hours  
**Impact:** Removes 1 untested module issue

---

### Phase 3: Subjective Reviews (~+2-3 points)

#### 3.1 Convention Drift Review [READY]
**Action:** `desloppify review --prepare --dimensions convention_outlier`  
**Current:** 82.0%  
**Status:** Review packet generated (20 batches ready)

#### 3.2 Test Strategy Review
**Action:** `desloppify review --prepare --dimensions test_strategy`  
**Current:** 68.5%  
**Target:** 95%

**Likely improvements:**
- Add integration tests for incremental builds
- Add benchmark tests for warm builds
- Add mock tests for service layer

#### 3.3 API Coherence Review
**Action:** `desloppify review --prepare --dimensions api_surface_coherence`  
**Current:** 72.5%

**Likely improvements:**
- Consolidate service interfaces
- Reduce exported surface area
- Document public APIs

---

## Detailed Action Plan

### Week 1: Test Foundation ✅ COMPLETE
| Day | Task | Status | Output |
|-----|------|--------|--------|
| 1 | Convention drift review | 🔄 Ready | Packet generated |
| 2-3 | Test async.go + retry.go | ✅ Complete | 27 tests, 53.5% coverage |
| 4-5 | Test search modules | ✅ Complete | 40+ tests, 80.1% coverage |

### Week 2: Core Coverage ⏳ IN PROGRESS
| Day | Task | Status | Output |
|-----|------|--------|--------|
| 1-2 | Test assets/wasm.go | ⏳ Pending | 1 issue resolved |
| 3-5 | Test parser/unified.go | ⏳ Pending | 1 issue resolved |
| 5 | Rescan and measure | ⏳ Pending | Score update |

### Week 3: Architectural ⏳ PENDING
| Day | Task | Status | Output |
|-----|------|--------|--------|
| 1-2 | Cache→Renderer decoupling | ⏳ Pending | Cross-module arch improved |
| 3-4 | API coherence review | ⏳ Pending | API coherence improved |
| 5 | Test strategy review | ⏳ Pending | Test strategy improved |

---

## Success Metrics

| Milestone | Target Score | Key Deliverables |
|-----------|--------------|------------------|
| After Tier 1-2 | 86-88/100 | 67+ tests, 53-80% coverage |
| After Week 2 | 88-90/100 | All 9 test coverage issues resolved |
| After Week 3 | 91-93/100 | Architectural improvements complete |
| Final | 95.0/100 | All subjective reviews complete |

---

## Risk Mitigation

### Risk: Test coverage doesn't move strict score
**Mitigation:** Focus on integration tests that verify build correctness, not just unit tests

### Risk: Architectural changes break builds
**Mitigation:**
- Run `go test ./...` after each change
- Run `kosh clean` on test site
- Verify dev mode works

### Risk: Subjective reviews don't improve scores
**Mitigation:** Focus on concrete improvements (test coverage, API consolidation) first

---

## Immediate Next Actions

**Completed:**
- ✅ Tier 1: Utils tests (27 tests, 53.5% coverage)
- ✅ Tier 2: Search tests (40+ tests, 80.1% coverage)

**Next Options:**
1. **Rescan now** - Measure progress after Tiers 1-2 (estimated +4-6 points)
2. **Continue Tier 2** - Test `wasm.go`, `unified.go` (remaining untested modules)
3. **Convention drift review** - Run subagent on 20 batches

---

## Commands Reference

```bash
# Review convention drift
desloppify review --prepare --dimensions convention_outlier
desloppify review --run-batches --dry-run

# Add tests
go test ./builder/utils/... -coverprofile=cover.out
go test ./builder/search/... -coverprofile=cover.out
go tool cover -html=cover.out

# Rescan after improvements
desloppify scan

# Check progress
desloppify status
desloppify show test_coverage
desloppify plan show
```

---

## Appendix: Issue Tracker

### T1-High Issues
- [ ] cache_renderer_upward_dep
- [x] channel_ownership_documented (positive pattern)
- [x] reconfigure_pattern_consolidated (positive pattern)
- [ ] search_complexity_concentration (acceptable isolation)
- [ ] utils_gravity_well
- [x] duplication_cache_commit_logic (verified - already centralized)
- [x] dual_sync_channels (Phase 2.1 - DONE)
- [x] fire_and_forget_error_logging (Phase 2.2 - DONE)
- [x] sink_stream_panic_safety (Phase 2.4 - DONE)

### T2-Medium Issues
- [ ] internal_build_coupling

### Test Coverage Issues (9 total)
- [x] builder/utils/async.go (65 LOC) - ✅ Tested
- [x] builder/utils/retry.go (82 LOC) - ✅ Tested
- [x] builder/search/fuzzy.go (365 LOC) - ✅ Tested
- [x] builder/search/stemmer.go (371 LOC) - ✅ Tested
- [ ] builder/assets/wasm.go (227 LOC) - ⏳ Pending
- [ ] builder/parser/unified.go (411 LOC) - ⏳ Pending
- [ ] cmd/kosh/cache_cmd.go (239 LOC) - ⏳ Pending
- [ ] cmd/kosh/ui.go (185 LOC) - ⏳ Pending

### Positive Patterns to Preserve
- ✅ Channel ownership documentation
- ✅ ReconfigureForBuild consolidation
- ✅ BuilderDependencies/BuilderState split
- ✅ AGENTS.md architectural contract
- ✅ FireAndForgetWithCleanup utility pattern

---

## Notes

- **21 stale tracked items** - may need `desloppify plan skip <id>` if no longer relevant
- **Test health 48.9% strict** - this is the breakthrough opportunity
- **Plateaued at 84.4** - structural changes need re-review to register
- **Focus on tests first** - highest ROI toward 95.0 target
- **67+ tests added** in Tiers 1-2, targeting 9 test coverage issues total
