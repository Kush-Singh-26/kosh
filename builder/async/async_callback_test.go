package async

import (
	"context"
	"errors"
	"sync"
	"testing"
)

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

func TestFireAndForgetWithCallback_NilCallback(t *testing.T) {
	ctx := context.Background()
	logger := helperLogger(t)

	var wg sync.WaitGroup
	wg.Add(1)

	expectedErr := errors.New("test error")

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
}

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
		OnError: func(_ error) {
			callbackCalled = true
		},
	})

	wg.Wait()

	if callbackCalled {
		t.Error("FireAndForgetWithCallback should not call onError on panic")
	}
}

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
		OnError: func(_ error) {
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
