package async

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// FireAndForget runs a function in a goroutine with standardized error handling.
// Errors are logged but don't propagate, suitable for background tasks like
// cache commits, social card generation, and other non-critical operations.
//
// Example:
//
//	FireAndForget(ctx, logger, "cache commit", func() error {
//	    return cache.BatchCommit(...)
//	})
func FireAndForget(ctx context.Context, logger *slog.Logger, operation string, fn func() error) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		if err := fn(); err != nil {
			logger.Error("Background operation failed",
				"operation", operation,
				"error", err)
		}
	}()
}

// FireAndForgetWithResult runs a function in a goroutine and returns an error channel.
// Callers can select on the channel to track completion/failure and implement retry logic.
// The channel is buffered (size 1) to prevent goroutine leaks if caller doesn't read.
// Errors are still logged in addition to being sent on the channel.
//
// Example:
//
//	errCh := FireAndForgetWithResult(ctx, logger, "cache commit", func() error {
//	    return cache.BatchCommit(...)
//	})
//	select {
//	case err := <-errCh:
//	    if err != nil { /* handle failure, maybe retry */ }
//	case <-time.After(5 * time.Second):
//	    /* timeout */
//	}
func FireAndForgetWithResult(ctx context.Context, logger *slog.Logger, operation string, fn func() error) <-chan error {
	errCh := make(chan error, 1) // buffered to prevent goroutine leak
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", r,
					"stack", string(debug.Stack()))
				errCh <- nil // signal completion even on panic
				return
			}
		}()

		err := fn()
		if err != nil {
			logger.Error("Background operation failed",
				"operation", operation,
				"error", err)
		}
		errCh <- err // always signal completion
	}()
	return errCh
}

// FireAndForgetWithCallback runs a function in a goroutine with error callback.
// The onError callback is invoked if the operation fails, allowing callers to
// track failures, increment metrics, or trigger alerts.
//
// Example:
//
//	FireAndForgetWithCallback(ctx, logger, "cache commit",
//	    func() error { return cache.BatchCommit(...) },
//	    func(err error) { metrics.Increment("cache_failures") })
func FireAndForgetWithCallback(ctx context.Context, logger *slog.Logger, operation string, fn func() error, onError func(error)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		if err := fn(); err != nil {
			logger.Error("Background operation failed",
				"operation", operation,
				"error", err)
			if onError != nil {
				onError(err)
			}
		}
	}()
}

// FireAndForgetWithMetrics runs a function in a goroutine with metrics tracking.
// The trackFailure callback is invoked on any error, enabling metrics collection
// for monitoring background operation failure rates.
//
// Example:
//
//	FireAndForgetWithMetrics(ctx, logger, "cache_commit",
//	    func() error { return cache.BatchCommit(...) },
//	    func() { metrics.Increment("background_cache_failures") })
func FireAndForgetWithMetrics(ctx context.Context, logger *slog.Logger, operation string, fn func() error, trackFailure func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		if err := fn(); err != nil {
			logger.Error("Background operation failed",
				"operation", operation,
				"error", err)
			if trackFailure != nil {
				trackFailure()
			}
		}
	}()
}

// FireAndForgetWithCleanup runs a function in a goroutine with cleanup callback.
// The cleanup function is always called, even on panic or error.
//
// Example:
//
//	FireAndForgetWithCleanup(ctx, logger, "social card",
//	    func() error { return generateCard() },
//	    func() { cleanupResources() })
func FireAndForgetWithCleanup(ctx context.Context, logger *slog.Logger, operation string, fn func() error, cleanup func()) {
	go func() {
		// Cleanup is always called, even on panic (registered first, runs last)
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		// Panic recovery (registered second, runs before cleanup)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", r,
					"stack", string(debug.Stack()))
			}
		}()

		if err := fn(); err != nil {
			logger.Error("Background operation failed",
				"operation", operation,
				"error", err)
		}
	}()
}
