package utils

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// testLogger creates a logger that writes to /dev/null for tests
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

func TestFireAndForget_Success(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var called bool
	var wg sync.WaitGroup

	wg.Add(1)
	FireAndForget(ctx, logger, "test operation", func() error {
		defer wg.Done()
		called = true
		return nil
	})

	// Wait for goroutine to complete
	wg.Wait()

	if !called {
		t.Error("Expected function to be called")
	}
}

func TestFireAndForget_LogsError(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var wg sync.WaitGroup

	wg.Add(1)
	errTest := errors.New("test error")

	FireAndForget(ctx, logger, "failing operation", func() error {
		defer wg.Done()
		return errTest
	})

	// Wait for goroutine to complete
	wg.Wait()
	// Error should be logged (no panic, test passes if no crash)
}

func TestFireAndForget_RecoverFromPanic(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var wg sync.WaitGroup

	wg.Add(1)

	FireAndForget(ctx, logger, "panicking operation", func() error {
		defer wg.Done()
		panic("test panic")
	})

	// Wait for goroutine to complete
	wg.Wait()
	// Panic should be recovered, test passes if no crash
}

func TestFireAndForget_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := testLogger(t)
	var wg sync.WaitGroup

	wg.Add(1)
	called := make(chan struct{}, 1)

	FireAndForget(ctx, logger, "context-aware operation", func() error {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			called <- struct{}{}
			return nil
		}
	})

	// Cancel immediately
	cancel()

	// Wait for goroutine to complete
	wg.Wait()

	// Verify context was checked
	select {
	case <-called:
		t.Error("Expected operation to respect context cancellation")
	default:
		// Expected - operation was cancelled
	}
}

func TestFireAndForgetWithCleanup_Success(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var called, cleaned bool
	var wg sync.WaitGroup

	wg.Add(1)
	FireAndForgetWithCleanup(ctx, logger, "test operation",
		func() error {
			defer wg.Done()
			called = true
			return nil
		},
		func() {
			cleaned = true
		})

	// Wait for goroutine to complete
	wg.Wait()

	if !called {
		t.Error("Expected function to be called")
	}
	if !cleaned {
		t.Error("Expected cleanup to be called")
	}
}

func TestFireAndForgetWithCleanup_Error(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var cleaned bool
	var wg sync.WaitGroup
	cleanedCh := make(chan struct{}, 1)

	wg.Add(1)
	errTest := errors.New("test error")

	FireAndForgetWithCleanup(ctx, logger, "failing operation",
		func() error {
			defer wg.Done()
			return errTest
		},
		func() {
			cleaned = true
			cleanedCh <- struct{}{}
		})

	// Wait for goroutine to complete
	wg.Wait()
	// Wait for cleanup to be called
	<-cleanedCh

	if !cleaned {
		t.Error("Expected cleanup to be called even on error")
	}
}

func TestFireAndForgetWithCleanup_Panic(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var cleaned bool
	var wg sync.WaitGroup
	cleanedCh := make(chan struct{}, 1)

	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "panicking operation",
		func() error {
			defer wg.Done()
			panic("test panic")
		},
		func() {
			cleaned = true
			cleanedCh <- struct{}{}
		})

	// Wait for goroutine to complete
	wg.Wait()
	// Wait for cleanup to be called
	<-cleanedCh

	if !cleaned {
		t.Error("Expected cleanup to be called even on panic")
	}
	// Panic should be recovered, test passes if no crash
}

func TestFireAndForgetWithCleanup_NilCleanup(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var called bool
	var wg sync.WaitGroup

	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "test operation",
		func() error {
			defer wg.Done()
			called = true
			return nil
		},
		nil) // nil cleanup function

	// Wait for goroutine to complete
	wg.Wait()

	if !called {
		t.Error("Expected function to be called")
	}
	// Should not panic with nil cleanup
}

func TestFireAndForgetWithCleanup_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := testLogger(t)
	var cleaned bool
	var wg sync.WaitGroup
	cleanedCh := make(chan struct{}, 1)

	wg.Add(1)

	FireAndForgetWithCleanup(ctx, logger, "context-aware operation",
		func() error {
			defer wg.Done()
			<-ctx.Done()
			return ctx.Err()
		},
		func() {
			cleaned = true
			cleanedCh <- struct{}{}
		})

	// Cancel immediately
	cancel()

	// Wait for goroutine to complete
	wg.Wait()
	// Wait for cleanup to be called
	<-cleanedCh

	if !cleaned {
		t.Error("Expected cleanup to be called even on context cancellation")
	}
}

func TestFireAndForget_Concurrent(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var wg sync.WaitGroup
	counter := 0
	var mu sync.Mutex

	// Launch 10 concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		FireAndForget(ctx, logger, "concurrent operation", func() error {
			defer wg.Done()
			mu.Lock()
			counter++
			mu.Unlock()
			return nil
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if counter != 10 {
		t.Errorf("Expected 10 operations, got %d", counter)
	}
}

func TestFireAndForgetWithCleanup_Concurrent(t *testing.T) {
	ctx := context.Background()
	logger := testLogger(t)
	var wg sync.WaitGroup
	counter := 0
	cleanupCounter := 0
	var mu sync.Mutex

	// Launch 10 concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		FireAndForgetWithCleanup(ctx, logger, "concurrent operation",
			func() error {
				defer wg.Done()
				mu.Lock()
				counter++
				mu.Unlock()
				return nil
			},
			func() {
				mu.Lock()
				cleanupCounter++
				mu.Unlock()
			})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if counter != 10 {
		t.Errorf("Expected 10 operations, got %d", counter)
	}
	if cleanupCounter != 10 {
		t.Errorf("Expected 10 cleanups, got %d", cleanupCounter)
	}
}
