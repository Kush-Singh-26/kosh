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
			if rec := recover(); rec != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", rec,
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
			if rec := recover(); rec != nil {
				logger.Error("Panic in background goroutine",
					"operation", operation,
					"panic", rec,
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

// FireAndForgetCallbackOptions configures FireAndForgetWithCallback.
type FireAndForgetCallbackOptions struct {
	// Required
	Ctx       context.Context
	Logger    *slog.Logger
	Operation string
	Fn        func() error

	// Optional
	OnError func(error)
}

// FireAndForgetWithCallback runs a function in a goroutine with error callback.
// The onError callback is invoked if the operation fails, allowing callers to
// track failures, increment metrics, or trigger alerts.
//
// Example:
//
//	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
//	    Ctx:       ctx,
//	    Logger:    logger,
//	    Operation: "cache commit",
//	    Fn:        func() error { return cache.BatchCommit(...) },
//	    OnError:   func(err error) { metrics.Increment("cache_failures") },
//	})
func FireAndForgetWithCallback(opts FireAndForgetCallbackOptions) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Fn == nil {
		panic("FireAndForgetWithCallback: Fn is nil")
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				opts.Logger.Error("Panic in background goroutine",
					"operation", opts.Operation,
					"panic", rec,
					"stack", string(debug.Stack()))
			}
		}()

		if err := opts.Fn(); err != nil {
			opts.Logger.Error("Background operation failed",
				"operation", opts.Operation,
				"error", err)
			if opts.OnError != nil {
				opts.OnError(err)
			}
		}
	}()
}

// FireAndForgetMetricsOptions configures FireAndForgetWithMetrics.
type FireAndForgetMetricsOptions struct {
	// Required
	Ctx       context.Context
	Logger    *slog.Logger
	Operation string
	Fn        func() error

	// Optional
	TrackFailure func()
}

// FireAndForgetWithMetrics runs a function in a goroutine with metrics tracking.
// The trackFailure callback is invoked on any error, enabling metrics collection
// for monitoring background operation failure rates.
//
// Example:
//
//	FireAndForgetWithMetrics(FireAndForgetMetricsOptions{
//	    Ctx:          ctx,
//	    Logger:       logger,
//	    Operation:    "cache_commit",
//	    Fn:           func() error { return cache.BatchCommit(...) },
//	    TrackFailure: func() { metrics.Increment("background_cache_failures") },
//	})
func FireAndForgetWithMetrics(opts FireAndForgetMetricsOptions) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Fn == nil {
		panic("FireAndForgetWithMetrics: Fn is nil")
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				opts.Logger.Error("Panic in background goroutine",
					"operation", opts.Operation,
					"panic", rec,
					"stack", string(debug.Stack()))
			}
		}()

		if err := opts.Fn(); err != nil {
			opts.Logger.Error("Background operation failed",
				"operation", opts.Operation,
				"error", err)
			if opts.TrackFailure != nil {
				opts.TrackFailure()
			}
		}
	}()
}

// FireAndForgetCleanupOptions configures FireAndForgetWithCleanup.
type FireAndForgetCleanupOptions struct {
	// Required
	Ctx       context.Context
	Logger    *slog.Logger
	Operation string
	Fn        func() error

	// Optional
	Cleanup func()
}

// FireAndForgetWithCleanup runs a function in a goroutine with cleanup callback.
// The cleanup function is always called, even on panic or error.
//
// Example:
//
//	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
//	    Ctx:       ctx,
//	    Logger:    logger,
//	    Operation: "social card",
//	    Fn:        func() error { return generateCard() },
//	    Cleanup:   func() { cleanupResources() },
//	})
func FireAndForgetWithCleanup(opts FireAndForgetCleanupOptions) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Fn == nil {
		panic("FireAndForgetWithCleanup: Fn is nil")
	}

	go func() {
		// Cleanup is always called, even on panic (registered first, runs last)
		defer func() {
			if opts.Cleanup != nil {
				opts.Cleanup()
			}
		}()

		// Panic recovery (registered second, runs before cleanup)
		defer func() {
			if rec := recover(); rec != nil {
				opts.Logger.Error("Panic in background goroutine",
					"operation", opts.Operation,
					"panic", rec,
					"stack", string(debug.Stack()))
			}
		}()

		if err := opts.Fn(); err != nil {
			opts.Logger.Error("Background operation failed",
				"operation", opts.Operation,
				"error", err)
		}
	}()
}
