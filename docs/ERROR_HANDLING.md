# Kosh Error Handling Strategy

This document outlines the engineering principles and patterns for error handling within the Kosh static site generator.

## Core Principles

### 1. Fail Fast for Structural Faults
Errors that prevent a correct or complete build must stop the process immediately. These include:
- Invalid configuration files.
- Missing required theme templates (`layout.html`, `index.html`).
- Inability to initialize the BoltDB cache.
- Staging directory creation failures.

### 2. Best-Effort for Optional Enhancements
Failures in non-critical paths should be logged but should not abort the build. These include:
- Social card generation (falls back to no image).
- SVG minification (falls back to original SVG).
- PWA icon generation (falls back to default icons or no PWA).
- SSR caching (falls back to re-rendering).

### 3. No Silent Failures
The use of `_ = ...` to ignore errors is strictly prohibited in production code unless:
- The error is wrapped in `buildctx.IgnoreError(err, "context")`, which logs it for debugging.
- The operation is a `defer` call where the error cannot be meaningfully handled (e.g., `defer file.Close()`), though even here, a log is preferred for writers.

### 4. Atomic Transitions
Builds must be atomic. If a clean build fails at any point before the final swap, the `staging` directory must be cleaned up, and the `output` directory must remain untouched.

### 5. Contextual Propagation
All asynchronous and service-layer operations must accept and respect a `context.Context`. This ensures that a canceled build stops all background work (workers, image processing, etc.) immediately.

## Common Patterns

### Error Wrapping
Always wrap errors to provide context while preserving the original error type for checks:
```go
if err != nil {
    return fmt.Errorf("failed to parse frontmatter for %s: %w", path, err)
}
```

### Parallel Error Handling
When using `errgroup`, ensure the context is passed to workers and the final `Wait()` is checked:
```go
g, ctx := errgroup.WithContext(buildCtx)
g.Go(func() error {
    return service.Process(ctx)
})
if err := g.Wait(); err != nil {
    return err
}
```

### Filesystem Retries
Windows filesystem operations can be flaky due to locking (e.g., AV scanners). Use the provided retry utilities for renames and deletions:
```go
if err := retry.RenameWithRetry(src, dst); err != nil {
    return fmt.Errorf("atomic publish failed: %w", err)
}
```

## Logging Levels
- **slog.Error**: Critical failure. The build or a major feature is broken.
- **slog.Warn**: Potential issue. The build continues, but some output may be degraded.
- **slog.Info**: Key build phases and user-facing progress.
- **slog.Debug**: Detailed internal state, including recovered/ignored non-fatal errors.
