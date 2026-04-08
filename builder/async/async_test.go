package async

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"log/slog"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/testutil"
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

	if !executed {
		t.Error("FireAndForget did not execute with cancelled context")
	}
}

// TestFireAndForgetWithCleanup_Success tests successful execution with cleanup
func TestFireAndForgetWithCleanup_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed, cleaned bool
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test success",
		Fn: func() error {
			mu.Lock()
			executed = true
			mu.Unlock()
			wg.Done()
			return nil
		},
		Cleanup: func() {
			mu.Lock()
			cleaned = true
			mu.Unlock()
			wg.Done()
		},
	})

	// Wait for goroutine to complete including cleanup
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
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
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	expectedErr := errors.New("test error")

	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test error",
		Fn: func() error {
			mu.Lock()
			executed = true
			mu.Unlock()
			wg.Done()
			return expectedErr
		},
		Cleanup: func() {
			mu.Lock()
			cleaned = true
			mu.Unlock()
			wg.Done()
		},
	})

	// Wait for goroutine to complete
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
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
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test panic",
		Fn: func() error {
			defer wg.Done()
			panic("test panic")
		},
		Cleanup: func() {
			mu.Lock()
			cleaned = true
			mu.Unlock()
			wg.Done()
		},
	})

	// Wait for goroutine to complete
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
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

	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test nil cleanup",
		Fn: func() error {
			executed = true
			wg.Done()
			return nil
		},
		Cleanup: nil, // nil cleanup
	})

	// Wait for goroutine to complete
	wg.Wait()

	if !executed {
		t.Error("FireAndForgetWithCleanup did not execute with nil cleanup")
	}
}

// TestFireAndForgetWithCleanup_CleanupRunsAfterPanic verifies cleanup runs even on panic
func TestFireAndForgetWithCleanup_CleanupRunsAfterPanic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	cleanupCalled := false
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test cleanup order",
		Fn: func() error {
			defer wg.Done()
			panic("intentional panic")
		},
		Cleanup: func() {
			mu.Lock()
			cleanupCalled = true
			mu.Unlock()
			wg.Done()
		},
	})

	// Wait for goroutine to complete
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
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
	wg.Add(numGoroutines * 2)

	for i := 0; i < numGoroutines; i++ {
		FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    logger,
			Operation: "test concurrent",
			Fn: func() error {
				mu.Lock()
				execCount++
				mu.Unlock()
				wg.Done()
				return nil
			},
			Cleanup: func() {
				mu.Lock()
				cleanupCount++
				mu.Unlock()
				wg.Done()
			},
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

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

	if !completed {
		t.Error("FireAndForget did not complete long-running task")
	}
}

// TestFireAndForget_ReturnsImmediately tests that FireAndForget returns immediately
func TestFireAndForget_ReturnsImmediately(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	start := time.Now()
	var executed atomic.Bool

	FireAndForget(ctx, logger, "test immediate", func() error {
		time.Sleep(100 * time.Millisecond)
		executed.Store(true)
		return nil
	})

	elapsed := time.Since(start)

	// FireAndForget should return immediately (within 1ms)
	if elapsed > 10*time.Millisecond {
		t.Errorf("FireAndForget did not return immediately: %v", elapsed)
	}

	// Ensure background task completes
	testutil.WaitForCondition(t, 200*time.Millisecond, func() bool {
		return executed.Load()
	})
}

// TestFireAndForgetWithCallback_Success tests successful execution without callback
func TestFireAndForgetWithCallback_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test success",
		Fn: func() error {
			executed = true
			wg.Done()
			return nil
		},
		OnError: nil,
	})

	wg.Wait()

	if !executed {
		t.Error("FireAndForgetWithCallback did not execute the function")
	}
}

// TestFireAndForgetWithCallback_Error tests error callback invocation
func TestFireAndForgetWithCallback_Error(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(2)
	var mu sync.Mutex
	var callbackCalled bool
	var callbackErr error

	expectedErr := errors.New("test error")

	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test error",
		Fn: func() error {
			wg.Done()
			return expectedErr
		},
		OnError: func(err error) {
			mu.Lock()
			callbackCalled = true
			callbackErr = err
			mu.Unlock()
			wg.Done()
		},
	})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !callbackCalled {
		t.Error("FireAndForgetWithCallback did not call onError")
	}
	if !errors.Is(callbackErr, expectedErr) {
		t.Errorf("FireAndForgetWithCallback callback error mismatch: got %v, want %v", callbackErr, expectedErr)
	}
}

// TestFireAndForgetWithCallback_NilCallback tests with nil error callback
func TestFireAndForgetWithCallback_NilCallback(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	expectedErr := errors.New("test error")

	// Should not panic with nil callback
	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test nil callback",
		Fn: func() error {
			defer wg.Done()
			return expectedErr
		},
		OnError: nil,
	})

	wg.Wait()
	// If we reach here, it worked correctly with nil callback
}

// TestFireAndForgetWithCallback_Panic tests panic recovery with callback
func TestFireAndForgetWithCallback_Panic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var callbackCalled bool

	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test panic",
		Fn: func() error {
			defer wg.Done()
			panic("test panic")
		},
		OnError: func(err error) {
			// Should not be called on panic
			callbackCalled = true
		},
	})

	wg.Wait()

	if callbackCalled {
		t.Error("FireAndForgetWithCallback should not call onError on panic")
	}
}

// TestFireAndForgetWithCallback_CallbackRunsAfterError verifies callback runs after error
func TestFireAndForgetWithCallback_CallbackRunsAfterError(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(2)
	var mu sync.Mutex
	var executionOrder []string

	expectedErr := errors.New("test error")

	FireAndForgetWithCallback(FireAndForgetCallbackOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test order",
		Fn: func() error {
			mu.Lock()
			executionOrder = append(executionOrder, "fn")
			mu.Unlock()
			wg.Done()
			return expectedErr
		},
		OnError: func(err error) {
			mu.Lock()
			executionOrder = append(executionOrder, "callback")
			mu.Unlock()
			wg.Done()
		},
	})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(executionOrder) != 2 {
		t.Fatalf("FireAndForgetWithCallback expected 2 executions, got %d", len(executionOrder))
	}
	if executionOrder[0] != "fn" {
		t.Errorf("FireAndForgetWithCallback expected fn first, got %s", executionOrder[0])
	}
	if executionOrder[1] != "callback" {
		t.Errorf("FireAndForgetWithCallback expected callback second, got %s", executionOrder[1])
	}
}

// TestFireAndForgetWithResult_Success tests successful execution with result channel
func TestFireAndForgetWithResult_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	errCh := FireAndForgetWithResult(ctx, logger, "test success", func() error {
		return nil
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FireAndForgetWithResult timed out")
	}
}

// TestFireAndForgetWithResult_Error tests error returned via channel
func TestFireAndForgetWithResult_Error(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	expectedErr := errors.New("test error")
	errCh := FireAndForgetWithResult(ctx, logger, "test error", func() error {
		return expectedErr
	})

	select {
	case err := <-errCh:
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FireAndForgetWithResult timed out")
	}
}

// TestFireAndForgetWithResult_Panic tests panic recovery with result channel
func TestFireAndForgetWithResult_Panic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	errCh := FireAndForgetWithResult(ctx, logger, "test panic", func() error {
		panic("test panic")
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected nil on panic, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FireAndForgetWithResult timed out")
	}
}

// TestFireAndForgetWithResult_ChannelBuffered tests that channel is buffered
func TestFireAndForgetWithResult_ChannelBuffered(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	errCh := FireAndForgetWithResult(ctx, logger, "test buffered", func() error {
		return errors.New("test error")
	})

	time.Sleep(50 * time.Millisecond)

	select {
	case <-errCh:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("FireAndForgetWithResult may have deadlocked - channel not buffered")
	}
}

// TestFireAndForgetWithMetrics_Success tests successful execution with metrics
func TestFireAndForgetWithMetrics_Success(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var tracked bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithMetrics(FireAndForgetMetricsOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test metrics",
		Fn: func() error {
			wg.Done()
			return nil
		},
		TrackFailure: func() {
			tracked = true
		},
	})

	wg.Wait()

	if tracked {
		t.Error("trackFailure should not be called on success")
	}
}

// TestFireAndForgetWithMetrics_Error tests metrics tracking on error
func TestFireAndForgetWithMetrics_Error(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var tracked bool
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	FireAndForgetWithMetrics(FireAndForgetMetricsOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test metrics error",
		Fn: func() error {
			wg.Done()
			return errors.New("test error")
		},
		TrackFailure: func() {
			mu.Lock()
			tracked = true
			mu.Unlock()
			wg.Done()
		},
	})

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !tracked {
		t.Error("trackFailure should be called on error")
	}
}

// TestFireAndForgetWithMetrics_NilTrack tests nil trackFailure callback
func TestFireAndForgetWithMetrics_NilTrack(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForgetWithMetrics(FireAndForgetMetricsOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "test nil track",
		Fn: func() error {
			wg.Done()
			return errors.New("test error")
		},
		TrackFailure: nil,
	})

	wg.Wait()
}
