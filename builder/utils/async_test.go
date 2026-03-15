package utils

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"log/slog"
)

// helperLogger creates a test logger
func helperLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestFireAndForget_Success tests successful execution
func TestFireAndForget_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test success", func() error {
		executed = true
		wg.Done()
		return nil
	})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond) // Allow logging to complete

	if !executed {
		t.Error("FireAndForget did not execute the function")
	}
}

// TestFireAndForget_Error tests error logging
func TestFireAndForget_Error(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	expectedErr := errors.New("test error")

	FireAndForget(ctx, logger, "test error", func() error {
		defer wg.Done()
		return expectedErr
	})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
}

// TestFireAndForget_Panic tests panic recovery
func TestFireAndForget_Panic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test panic", func() error {
		defer wg.Done()
		panic("test panic")
	})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	// If we reach here, panic was recovered successfully
}

// TestFireAndForget_ContextCancellation tests behavior with cancelled context
func TestFireAndForget_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	logger := helperLogger(t)

	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test cancelled", func() error {
		executed = true
		wg.Done()
		// Check context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !executed {
		t.Error("FireAndForget did not execute with cancelled context")
	}
}

// TestFireAndForgetWithCleanup_Success tests successful execution with cleanup
func TestFireAndForgetWithCleanup_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed, cleaned bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "test success",
		func() error {
			executed = true
			wg.Done()
			return nil
		},
		func() {
			cleaned = true
		})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !executed {
		t.Error("FireAndForgetWithCleanup did not execute the function")
	}
	if !cleaned {
		t.Error("FireAndForgetWithCleanup did not call cleanup")
	}
}

// TestFireAndForgetWithCleanup_Error tests error case with cleanup
func TestFireAndForgetWithCleanup_Error(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed, cleaned bool
	var wg sync.WaitGroup
	wg.Add(1)

	expectedErr := errors.New("test error")

	FireAndForgetWithCleanup(ctx, logger, "test error",
		func() error {
			executed = true
			defer wg.Done()
			return expectedErr
		},
		func() {
			cleaned = true
		})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !executed {
		t.Error("FireAndForgetWithCleanup did not execute the function")
	}
	if !cleaned {
		t.Error("FireAndForgetWithCleanup did not call cleanup on error")
	}
}

// TestFireAndForgetWithCleanup_Panic tests panic recovery with cleanup
func TestFireAndForgetWithCleanup_Panic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var cleaned bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "test panic",
		func() error {
			defer wg.Done()
			panic("test panic")
		},
		func() {
			cleaned = true
		})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !cleaned {
		t.Error("FireAndForgetWithCleanup did not call cleanup on panic")
	}
	// If we reach here, panic was recovered successfully
}

// TestFireAndForgetWithCleanup_NilCleanup tests with nil cleanup function
func TestFireAndForgetWithCleanup_NilCleanup(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "test nil cleanup",
		func() error {
			executed = true
			wg.Done()
			return nil
		},
		nil) // nil cleanup

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !executed {
		t.Error("FireAndForgetWithCleanup did not execute with nil cleanup")
	}
}

// TestFireAndForgetWithCleanup_CleanupRunsAfterPanic verifies cleanup runs even on panic
func TestFireAndForgetWithCleanup_CleanupRunsAfterPanic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	cleanupCalled := false
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "test cleanup order",
		func() error {
			defer wg.Done()
			panic("intentional panic")
		},
		func() {
			cleanupCalled = true
		})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !cleanupCalled {
		t.Error("Cleanup was not called after panic")
	}
}

// TestFireAndForget_MultipleGoroutines tests concurrent execution
func TestFireAndForget_MultipleGoroutines(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	numGoroutines := 10
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		FireAndForget(ctx, logger, "test concurrent", func() error {
			mu.Lock()
			counter++
			mu.Unlock()
			wg.Done()
			return nil
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if counter != numGoroutines {
		t.Errorf("FireAndForget expected %d executions, got %d", numGoroutines, counter)
	}
}

// TestFireAndForgetWithCleanup_MultipleGoroutines tests concurrent execution with cleanup
func TestFireAndForgetWithCleanup_MultipleGoroutines(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var execCount, cleanupCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	numGoroutines := 10
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		FireAndForgetWithCleanup(ctx, logger, "test concurrent",
			func() error {
				mu.Lock()
				execCount++
				mu.Unlock()
				wg.Done()
				return nil
			},
			func() {
				mu.Lock()
				cleanupCount++
				mu.Unlock()
			})
	}

	// Wait for all goroutines to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if execCount != numGoroutines {
		t.Errorf("FireAndForgetWithCleanup expected %d executions, got %d", numGoroutines, execCount)
	}
	if cleanupCount != numGoroutines {
		t.Errorf("FireAndForgetWithCleanup expected %d cleanups, got %d", numGoroutines, cleanupCount)
	}
}

// TestFireAndForget_LongRunning tests long-running background task
func TestFireAndForget_LongRunning(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var completed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test long running", func() error {
		defer wg.Done()
		// Simulate long-running task
		time.Sleep(50 * time.Millisecond)
		completed = true
		return nil
	})

	// Wait for goroutine to complete
	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	if !completed {
		t.Error("FireAndForget did not complete long-running task")
	}
}

// TestFireAndForget_ReturnsImmediately tests that FireAndForget returns immediately
func TestFireAndForget_ReturnsImmediately(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	start := time.Now()

	FireAndForget(ctx, logger, "test immediate", func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	elapsed := time.Since(start)

	// FireAndForget should return immediately (within 1ms)
	if elapsed > time.Millisecond {
		t.Errorf("FireAndForget did not return immediately: %v", elapsed)
	}
}
