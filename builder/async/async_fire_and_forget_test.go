package async

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

	wg.Wait()

	if !executed {
		t.Error("FireAndForget did not execute the function")
	}
}

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

	wg.Wait()
}

func TestFireAndForget_Panic(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test panic", func() error {
		defer wg.Done()
		panic("test panic")
	})

	wg.Wait()
}

func TestFireAndForget_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger := helperLogger(t)

	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test cancelled", func() error {
		executed = true
		wg.Done()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	wg.Wait()

	if !executed {
		t.Error("FireAndForget did not execute with cancelled context")
	}
}

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

	wg.Wait()

	if counter != numGoroutines {
		t.Errorf("FireAndForget expected %d executions, got %d", numGoroutines, counter)
	}
}

func TestFireAndForget_LongRunning(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var completed bool
	var wg sync.WaitGroup
	wg.Add(1)

	FireAndForget(ctx, logger, "test long running", func() error {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		completed = true
		return nil
	})

	wg.Wait()

	if !completed {
		t.Error("FireAndForget did not complete long-running task")
	}
}

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

	if elapsed > 10*time.Millisecond {
		t.Errorf("FireAndForget did not return immediately: %v", elapsed)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return executed.Load()
	})
}

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

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !cleaned {
		t.Error("FireAndForgetWithCleanup did not call cleanup on panic")
	}
}

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
		Cleanup: nil,
	})

	wg.Wait()

	if !executed {
		t.Error("FireAndForgetWithCleanup did not execute with nil cleanup")
	}
}

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

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if !cleanupCalled {
		t.Error("Cleanup was not called after panic")
	}
}

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

	wg.Wait()

	if execCount != numGoroutines {
		t.Errorf("FireAndForgetWithCleanup expected %d executions, got %d", numGoroutines, execCount)
	}
	if cleanupCount != numGoroutines {
		t.Errorf("FireAndForgetWithCleanup expected %d cleanups, got %d", numGoroutines, cleanupCount)
	}
}
