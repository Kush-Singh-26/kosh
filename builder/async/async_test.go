package async

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestFireAndForget_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var executed int32
	FireAndForget(ctx, slog.Default(), "test", func() error {
		atomic.StoreInt32(&executed, 1)
		return nil
	})

	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&executed) == 1 {
		t.Error("Fn should not have executed on canceled context")
	}
}

func TestFireAndForgetWithResult_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var executed int32
	errCh := FireAndForgetWithResult(ctx, slog.Default(), "test", func() error {
		atomic.StoreInt32(&executed, 1)
		return nil
	})

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for error channel")
	}

	if atomic.LoadInt32(&executed) == 1 {
		t.Error("Fn should not have executed on canceled context")
	}
}

func TestFireAndForgetWithCleanup_AlwaysCleansUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var cleanedUp int32
	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    slog.Default(),
		Operation: "test",
		Fn:        func() error { return nil },
		Cleanup: func() {
			atomic.StoreInt32(&cleanedUp, 1)
		},
	})

	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&cleanedUp) != 1 {
		t.Error("Cleanup should have executed even on canceled context")
	}
}

func TestFireAndForgetWithCleanup_PanicCleanup(t *testing.T) {
	var cleanedUp int32
	FireAndForgetWithCleanup(FireAndForgetCleanupOptions{
		Ctx:       context.Background(),
		Logger:    slog.Default(),
		Operation: "test",
		Fn: func() error {
			panic("test panic")
		},
		Cleanup: func() {
			atomic.StoreInt32(&cleanedUp, 1)
		},
	})

	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&cleanedUp) != 1 {
		t.Error("Cleanup should have executed even on panic")
	}
}
