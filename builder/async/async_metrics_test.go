package async

import (
	"context"
	"errors"
	"sync"
	"testing"
)

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
