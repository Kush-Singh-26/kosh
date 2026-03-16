package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerPool_PanicRecovery verifies that worker pools recover from panics
func TestWorkerPool_PanicRecovery(t *testing.T) {
	ctx := context.Background()
	var processed atomic.Int32

	pool := NewWorkerPool(ctx, 4, func(task int) {
		if task == 5 {
			panic("intentional panic for testing")
		}
		processed.Add(1)
	})

	pool.Start()

	// Submit tasks including one that will panic
	for i := 0; i < 10; i++ {
		pool.Submit(i)
	}

	// Give time for processing
	time.Sleep(100 * time.Millisecond)
	pool.Stop()

	// Should have processed 9 tasks (all except the panicking one)
	// The panic should not crash the pool
	if processed.Load() < 8 {
		t.Errorf("Expected at least 8 tasks processed, got %d", processed.Load())
	}
}

// TestWorkerPool_SchedulerTokenReleaseOnCancel verifies scheduler tokens are released on context cancellation
func TestWorkerPool_SchedulerTokenReleaseOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var acquired, released atomic.Int32

	mockScheduler := &mockScheduler{
		acquire: func(ctx context.Context, task TaskType) error {
			acquired.Add(1)
			// Block until context is cancelled
			<-ctx.Done()
			return ctx.Err()
		},
		release: func(task TaskType) {
			released.Add(1)
		},
	}

	pool := NewWorkerPool(ctx, 2, func(task int) {
		time.Sleep(50 * time.Millisecond)
	})
	pool.WithScheduler(mockScheduler, TaskDefault)
	pool.Start()

	// Submit tasks
	for i := 0; i < 5; i++ {
		pool.Submit(i)
	}

	// Cancel context while tasks are in flight
	time.Sleep(20 * time.Millisecond)
	cancel()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify tokens were released (at least some of them)
	if released.Load() < acquired.Load()-2 {
		t.Errorf("Expected tokens to be released, acquired=%d, released=%d", acquired.Load(), released.Load())
	}
}

// TestWorkerPool_StopWaitsForInFlightTasks verifies Stop() waits for in-flight tasks
func TestWorkerPool_StopWaitsForInFlightTasks(t *testing.T) {
	ctx := context.Background()
	var completed atomic.Int32

	pool := NewWorkerPool(ctx, 2, func(task int) {
		time.Sleep(50 * time.Millisecond)
		completed.Add(1)
	})

	pool.Start()

	// Submit tasks
	for i := 0; i < 4; i++ {
		pool.Submit(i)
	}

	// Stop immediately - should wait for completion
	start := time.Now()
	pool.Stop()
	elapsed := time.Since(start)

	// Should have waited for at least some tasks to complete
	if elapsed < 50*time.Millisecond {
		t.Errorf("Stop() returned too quickly (%v), should have waited for in-flight tasks", elapsed)
	}

	if completed.Load() < 2 {
		t.Errorf("Expected at least 2 tasks completed, got %d", completed.Load())
	}
}

// TestWorkerPool_SubmitAfterStop verifies Submit is safe after Stop
func TestWorkerPool_SubmitAfterStop(t *testing.T) {
	ctx := context.Background()
	var processed atomic.Int32

	pool := NewWorkerPool(ctx, 2, func(task int) {
		processed.Add(1)
	})

	pool.Start()
	pool.Submit(1)
	pool.Stop()

	// Submit after stop should be safe (no panic, no deadlock)
	pool.Submit(2)
	pool.Submit(3)

	// Give time for any potential processing
	time.Sleep(50 * time.Millisecond)

	// Only the first task should have been processed
	if processed.Load() != 1 {
		t.Errorf("Expected exactly 1 task processed, got %d", processed.Load())
	}
}

// TestWorkerPool_ConcurrentSubmit verifies thread-safe concurrent submission
func TestWorkerPool_ConcurrentSubmit(t *testing.T) {
	ctx := context.Background()
	var processed atomic.Int32

	pool := NewWorkerPool(ctx, 4, func(task int) {
		processed.Add(1)
	})

	pool.Start()

	// Concurrent submissions from multiple goroutines
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				pool.Submit(base*10 + i)
			}
		}(g)
	}

	wg.Wait()
	pool.Stop()

	if processed.Load() != 100 {
		t.Errorf("Expected 100 tasks processed, got %d", processed.Load())
	}
}

// TestWorkerPool_ContextCancellation verifies tasks respect context cancellation
func TestWorkerPool_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var processed atomic.Int32

	pool := NewWorkerPool(ctx, 2, func(task int) {
		time.Sleep(20 * time.Millisecond)
		processed.Add(1)
	})

	pool.Start()

	// Submit more tasks than can be processed in time
	for i := 0; i < 20; i++ {
		pool.Submit(i)
	}

	// Wait for context to expire and pool to stop
	time.Sleep(150 * time.Millisecond)

	// Some tasks should have been processed, but not all
	count := processed.Load()
	if count <= 0 {
		t.Error("Expected at least some tasks to be processed before context cancellation")
	}
	if count >= 20 {
		t.Error("Expected context cancellation to prevent some tasks from processing")
	}
}

// mockScheduler is a test double for BuildScheduler
type mockScheduler struct {
	acquire func(ctx context.Context, task TaskType) error
	release func(task TaskType)
}

func (m *mockScheduler) Acquire(ctx context.Context, task TaskType) error {
	if m.acquire != nil {
		return m.acquire(ctx, task)
	}
	return nil
}

func (m *mockScheduler) Release(task TaskType) {
	if m.release != nil {
		m.release(task)
	}
}
