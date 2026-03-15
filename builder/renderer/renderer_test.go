package renderer

import (
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

)

func setupRendererTest(t *testing.T) *Renderer {
	t.Helper()

	sink := testutil.NewMemSink()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	return &Renderer{
		Sink:        sink,
		Assets:      make(map[string]string),
		RenderedSet: make(map[string]bool),
		Compress:    false,
		logger:      logger,
	}
}

func TestRenderer_recordError(t *testing.T) {
	r := setupRendererTest(t)

	// Record some errors
	r.recordError("test error 1", "/path/to/file1.html", errors.New("error 1"))
	r.recordError("test error 2", "/path/to/file2.html", errors.New("error 2"))
	r.recordError("test error 3", "/path/to/file3.html", errors.New("error 3"))

	// Verify errors are stored
	r.errMu.Lock()
	storedCount := len(r.renderErrors)
	r.errMu.Unlock()

	if storedCount != 3 {
		t.Errorf("Expected 3 stored errors, got %d", storedCount)
	}
}

func TestRenderer_ConsumeErrors(t *testing.T) {
	r := setupRendererTest(t)

	// Initially should return nil
	errList := r.ConsumeErrors()
	if errList != nil {
		t.Error("ConsumeErrors should return nil when no errors stored")
	}

	// Record some errors
	r.recordError("failed to close file", "/path/file1.html", errors.New("close error"))
	r.recordError("failed to flush buffer", "/path/file2.html", errors.New("flush error"))

	// Get errors
	errList = r.ConsumeErrors()
	if len(errList) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(errList))
	}

	// Verify error messages contain expected content
	foundClose := false
	foundFlush := false
	for _, err := range errList {
		errStr := err.Error()
		if strings.Contains(errStr, "failed to close file") {
			foundClose = true
		}
		if strings.Contains(errStr, "failed to flush buffer") {
			foundFlush = true
		}
		if !strings.Contains(errStr, "/path/file") {
			t.Error("Error should contain file path")
		}
	}

	if !foundClose {
		t.Error("Should find 'failed to close file' error")
	}
	if !foundFlush {
		t.Error("Should find 'failed to flush buffer' error")
	}

	// Verify errors are cleared after retrieval
	errList2 := r.ConsumeErrors()
	if errList2 != nil {
		t.Error("ConsumeErrors should return nil after errors are retrieved and cleared")
	}
}

func TestRenderer_ConsumeErrors_ConcurrentAccess(t *testing.T) {
	r := setupRendererTest(t)

	// Record errors from multiple goroutines
	done := make(chan bool, 3)

	go func() {
		r.recordError("error from goroutine 1", "/path/1.html", errors.New("err1"))
		done <- true
	}()

	go func() {
		r.recordError("error from goroutine 2", "/path/2.html", errors.New("err2"))
		done <- true
	}()

	go func() {
		r.recordError("error from goroutine 3", "/path/3.html", errors.New("err3"))
		done <- true
	}()

	// Wait for all goroutines
	for range 3 {
		<-done
	}

	// Verify all errors were recorded
	errList := r.ConsumeErrors()
	if len(errList) != 3 {
		t.Errorf("Expected 3 errors from concurrent access, got %d", len(errList))
	}
}

func TestRenderer_ConsumeErrors_ClearAfterRetrieval(t *testing.T) {
	r := setupRendererTest(t)

	// Record an error
	r.recordError("test error", "/path/test.html", errors.New("test"))

	// First retrieval should return the error
	errList1 := r.ConsumeErrors()
	if len(errList1) != 1 {
		t.Errorf("First retrieval: expected 1 error, got %d", len(errList1))
	}

	// Second retrieval should return nil (errors cleared)
	errList2 := r.ConsumeErrors()
	if errList2 != nil {
		t.Errorf("Second retrieval: expected nil, got %d errors", len(errList2))
	}

	// Recording new error should work after clear
	r.recordError("new error", "/path/new.html", errors.New("new"))
	errList3 := r.ConsumeErrors()
	if len(errList3) != 1 {
		t.Errorf("After new error: expected 1 error, got %d", len(errList3))
	}
}
