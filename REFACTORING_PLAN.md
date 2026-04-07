# Refactoring Plan: Code Quality & Robustness

This document outlines the detailed plan to address code quality issues, function parameter bloat, "AI slop," and swallowed errors across the Kosh codebase.

## Phase 1: Taming the "God Functions" & Parameter Bloat
**Goal:** Improve readability and testability by breaking down massive functions and grouping parameter lists into cohesive configuration structs.

### 1.1 Complete Parameter Bloat Fix in `builder/services/post/post_service.go`
* **Problem:** Streaming worker functions like `runStreamingParsePhase` and `parseWorkerTaskStreaming` take 8-12 parameters each.
* **Changes:**
  * Define a `WorkerContext` struct to hold shared dependencies.
    ```go
    type WorkerContext struct {
        Ctx                context.Context
        PC                 *postProcessContext
        CardPool           *async.WorkerPool[socialCardTask]
        SearchPool         *async.WorkerPool[searchTask]
        RenderChan         chan<- renderTask
        ShouldForce        bool
        ForceSocialRebuild bool
    }
    ```
  * Refactor `runStreamingParsePhase` signature:
    `func (s *postService) runStreamingParsePhase(numWorkers int, files []models.ScannedFile, wCtx WorkerContext) error`
  * Refactor `parseWorkerTaskStreaming` signature:
    `func (s *postService) parseWorkerTaskStreaming(f models.ScannedFile, wCtx WorkerContext)`
  * Refactor `aggregateAndStream` signature to use `WorkerContext`.

## Phase 2: Eliminating "AI Slop" & Constructor Duplication
**Goal:** Remove auto-generated boilerplate, standardize instantiation, and improve documentation for complex custom logic.

### 2.1 Unify Constructors in `builder/orchestration/engine.go`
* **Problem:** Redundant constructor variants still exist even though functional options have been partially introduced.
* **Changes:**
  * Ensure the Functional Options pattern is fully utilized:
    ```go
    type EngineOption func(*engineOptions)
    func WithConfig(cfg *config.Config) EngineOption
    func WithFs(vfs afero.Fs) EngineOption
    func WithReporter(r ui.Reporter) EngineOption
    func WithDeps(deps EngineDependencies) EngineOption
    func NewEngine(opts ...EngineOption) (*Engine, error)
    ```
  * Remove legacy standalone functions: `NewEngineFromManual`, `newEngineFromManual`, `newEngineWithConfigFs` (if applicable), and ensure all instantiation goes through `NewEngine`.

### 2.2 Document Fragile Parsers in `builder/search/core/fuzzy.go`
* **Problem:** Manual rune-based parsers like `ParseQuery` lack documentation.
* **Changes:**
  * Add precise Go doc comments to `ParseQuery` documenting the expected grammar (handling of `+`, `-`, and quoted phrases) and edge cases. Comments should follow idiomatic Go documentation patterns.

## Phase 3: Address Linting Issues
**Goal:** Fix all 14 issues reported by `golangci-lint run ./...` to maintain codebase hygiene.

### 3.1 Unused Code
* `builder/orchestration/logger.go`: Remove unused constants `cyan`, `green`, `yellow`, `red`, `dim`, `reset`, `bold` and variable `devTimeFormat`.
* `builder/assets/pipeline.go`: Remove unused `originalRelPath` field from the `fileTask` struct.

### 3.2 Unchecked Errors (`errcheck`)
* `builder/renderer/template_cache_test.go`: Handle the returned error from `r.ReloadTemplates()` instead of ignoring it.
* `builder/ui/pterm_reporter.go`: Handle the returned error from `r.area.Stop()` and `Render()`.

### 3.3 Code Simplification (`staticcheck`)
* `builder/services/asset/asset_service.go`: Apply De Morgan's law on line 131: `if !(skipImages && s.cfg.IsDev) {` -> `if !skipImages || !s.cfg.IsDev {`.

## Phase 4: Improve Test Coverage in Core Orchestration
**Goal:** Add test coverage to critical paths that currently have 0% coverage.

### 4.1 Missing Coverage Areas
* `builder/orchestration/assets`
* `builder/orchestration/incremental`
* `builder/orchestration/search`
* `builder/orchestration/watch`
* `builder/services/wasm`

### 4.2 Actions
* Create basic `_test.go` files for each of these packages.
* Mock necessary dependencies to ensure unit tests are isolated and fast.
* Verify coverage improvements using `go test -short -cover ./...`.
