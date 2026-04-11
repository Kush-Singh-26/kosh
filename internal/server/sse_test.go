package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testResponseWriter struct {
	header http.Header
	code   int
}

func (w *testResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *testResponseWriter) Write(data []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return len(data), nil
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func TestHandleSSERequiresFlusher(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := &testResponseWriter{}

	handleSSE(rr, req)

	if rr.code != http.StatusInternalServerError {
		t.Fatalf("handleSSE() status = %d, want %d", rr.code, http.StatusInternalServerError)
	}
}

func TestBroadcastReload_Functional(t *testing.T) {
	// Register a mock client
	clientChan := make(chan string, 1)
	clientMu.Lock()
	clients[clientChan] = struct{}{}
	clientMu.Unlock()

	defer func() {
		clientMu.Lock()
		delete(clients, clientChan)
		clientMu.Unlock()
	}()

	broadcastReload("site")

	select {
	case target := <-clientChan:
		if target != "site" {
			t.Errorf("Expected target 'site', got %s", target)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timed out waiting for reload broadcast")
	}
}
