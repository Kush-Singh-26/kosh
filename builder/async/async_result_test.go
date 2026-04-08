package async

import (
	"context"
	"errors"
	"testing"
	"time"
)

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

func TestFireAndForgetWithResult_ChannelBuffered(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	errCh := FireAndForgetWithResult(ctx, logger, "test buffered", func() error {
		return errors.New("test error")
	})

	time.Sleep(50 * time.Millisecond)

	select {
	case <-errCh:
	case <-time.After(1 * time.Second):
		t.Fatal("FireAndForgetWithResult may have deadlocked - channel not buffered")
	}
}
