# Action Plan: Road to 95.0 (Phase 3)

**Current Status:** 84.4/100 strict (target: 95.0, need +10.6)
**Created:** 2026-03-15
**Commit:** 64251f4 (Phase 2 complete)

---

## Executive Summary

### Score Analysis
| Dimension | Score | Gap | Weight | Impact |
|-----------|-------|-----|--------|--------|
| **Test health (strict)** | 48.9% | -46.1% | High | 🔴 Biggest breakthrough opportunity |
| **Mid elegance** | 78.5% | -21.5% | 17.9% | 🔴 -2.88 pts drag |
| **High elegance** | 88.5% | -11.5% | 17.9% | 🟡 -1.54 pts drag |
| **Test strategy** | 68.5% | -26.5% | High | 🔴 Subjective drag |
| **AI generated debt** | 71.0% | -24.0% | Medium | 🟡 Subjective drag |
| **API coherence** | 72.5% | -22.5% | Medium | 🟡 Subjective drag |

### Key Insight
**Test health (48.9% strict)** is explicitly noted by desloppify as "where the breakthrough is."
Improving test coverage could yield +5-10 points toward the 95.0 target.

---

## Priority Tiers

### Tier 1: Quick Wins (1-2 days, +2-4 points estimated)

#### 1.1 Address Convention Drift Queue Item
**Action:** `desloppify review --prepare --dimensions convention_outlier`
**Effort:** 2-4 hours
**Impact:** Clears queue, may improve convention drift score (82.0%)
**Files:** TBD by review

#### 1.2 Add Tests for New Phase 2 Code
**Files:**
- `builder/utils/async.go` (65 LOC) - FireAndForget utilities
- `builder/utils/retry.go` (82 LOC) - Retry functions

**Tests to add:**
```go
// async_test.go
func TestFireAndForget_LogsError(t *testing.T)
func TestFireAndForgetWithCleanup_CallsCleanup(t *testing.T)
func TestFireAndForgetWithCleanup_RecoverFromPanic(t *testing.T)

// retry_test.go
func TestRenameWithRetry_Success(t *testing.T)
func TestRenameWithRetry_ExhaustsRetries(t *testing.T)
func TestRemoveAllWithRetry_Success(t *testing.T)
```

**Effort:** 2-3 hours
**Impact:** Removes 2 untested module issues

---

### Tier 2: High-Impact Test Coverage (3-5 days, +4-6 points estimated)

#### 2.1 Test Critical Search Modules
**Priority:** T2 (high blast radius)

**Files:**
- `builder/search/fuzzy.go` (365 LOC) - Fuzzy search algorithm
- `builder/search/stemmer.go` (371 LOC) - Stemming logic

**Tests to add:**
```go
// fuzzy_test.go
func TestFuzzyMatch_ScoreCalculation(t *testing.T)
func TestFuzzyMatch_EdgeCases(t *testing.T)
func TestFuzzyMatch_Performance(t *testing.T)

// stemmer_test.go
func TestStemmer_EnglishRules(t *testing.T)
func TestStemmer_EdgeCases(t *testing.T)
func TestStemmer_Consistency(t *testing.T)
```

**Effort:** 4-6 hours
**Impact:** Removes 2 untested critical issues, improves search reliability

#### 2.2 Test Core Infrastructure
**Files:**
- `builder/assets/wasm.go` (227 LOC) - WASM compile/deploy
- `builder/parser/unified.go` (411 LOC) - Markdown parsing

**Tests to add:**
```go
// wasm_test.go
func TestCheckWASM_DeploysEmbedded(t *testing.T)
func TestCompileWASMFromSource_Success(t *testing.T)
func TestDeployWASMFromFile(t *testing.T)

// unified_test.go
func TestUnifiedParser_FrontmatterExtraction(t *testing.T)
func TestUnifiedParser_BodyParsing(t *testing.T)
func TestUnifiedParser_MathExpressions(t *testing.T)
```

**Effort:** 6-8 hours
**Impact:** Removes 2 untested module issues

---

### Tier 3: Architectural Improvements (5-8 days, +3-5 points estimated)

#### 3.1 Phase 1.1: Cache→Renderer Decoupling
**Issue:** Cache stores SSR types that import renderer
**Files:** `builder/cache/types.go`, `builder/models/ssr.go`

**Steps:**
1. Move SSR types to `builder/models/ssr.go` (already exists, verify completeness)
2. Update cache to use opaque blob storage
3. Update adapter to use new types

**Effort:** 3-4 hours
**Impact:** Improves cross-module arch (72.5%), design coherence (82.5%)

#### 3.2 Phase 1.2: WASM Decoupling
**Issue:** `builder/run` imports `internal/build`
**Files:** `builder/run/builder.go`, `internal/build/build.go`

**Steps:**
1. Move WASM deployment to `builder/assets/` (already done in Phase 1.2)
2. Update imports

**Status:** ✅ Already completed in Phase 1.2

---

### Tier 4: Subjective Review Items (2-3 days, +2-3 points estimated)

#### 4.1 Test Strategy Review
**Action:** `desloppify review --prepare --dimensions test_strategy`
**Current:** 68.5%
**Target:** 95%

**Likely improvements:**
- Add integration tests for incremental builds
- Add benchmark tests for warm builds
- Add mock tests for service layer

#### 4.2 API Coherence Review
**Action:** `desloppify review --prepare --dimensions api_surface_coherence`
**Current:** 72.5%

**Likely improvements:**
- Consolidate service interfaces
- Reduce exported surface area
- Document public APIs

---

## Detailed Action Plan

### Week 1: Test Foundation
| Day | Task | Expected Output |
|-----|------|-----------------|
| 1 | Convention drift review + fix | Queue cleared |
| 2-3 | Test async.go + retry.go | 2 issues resolved |
| 4-5 | Test search/fuzzy.go + stemmer.go | 2 critical issues resolved |

### Week 2: Core Coverage
| Day | Task | Expected Output |
|-----|------|-----------------|
| 1-2 | Test assets/wasm.go | 1 issue resolved |
| 3-5 | Test parser/unified.go | 1 issue resolved |
| 5 | Rescan and measure | Score update |

### Week 3: Architectural
| Day | Task | Expected Output |
|-----|------|-----------------|
| 1-2 | Cache→Renderer decoupling | Cross-module arch improved |
| 3-4 | API coherence review | API coherence improved |
| 5 | Test strategy review | Test strategy improved |

---

## Success Metrics

| Milestone | Target Score | Key Deliverables |
|-----------|--------------|------------------|
| After Week 1 | 86-87/100 | Queue cleared, 4 test issues resolved |
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

1. **Today:** Run `desloppify review --prepare --dimensions convention_outlier`
2. **Tomorrow:** Add tests for `builder/utils/async.go`
3. **This week:** Complete Tier 1 (quick wins)

---

## Commands Reference

```bash
# Review convention drift
desloppify review --prepare --dimensions convention_outlier
desloppify review --run-batches --dry-run

# Add tests (manual)
go test ./builder/utils/... -coverprofile=cover.out
go tool cover -html=cover.out

# Rescan after improvements
desloppify scan

# Check progress
desloppify status
desloppify show test_coverage
```

---

## Notes

- **21 stale tracked items** - may need `desloppify plan skip <id>` if no longer relevant
- **Test health 48.9% strict** - this is the breakthrough opportunity
- **Plateaued at 84.4** - structural changes need re-review to register
- **Focus on tests first** - highest ROI toward 95.0 target
