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

func TestBroadcastReloadStopsWhenChannelCloses(t *testing.T) {
	done := make(chan struct{})
	ch := make(chan struct{})

	go func() {
		broadcastReload(ch)
		close(done)
	}()

	close(ch)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("broadcastReload did not exit after channel close")
	}
}

func TestBroadcastReload_Functional(t *testing.T) {
	ch := make(chan struct{})
	go func() {
		// Wait a bit to ensure clients are registered
		time.Sleep(50 * time.Millisecond)
		ch <- struct{}{}
		close(ch)
	}()

	// Register a mock client
	clientChan := make(chan struct{}, 1)
	clientMu.Lock()
	clients[clientChan] = struct{}{}
	clientMu.Unlock()

	defer func() {
		clientMu.Lock()
		delete(clients, clientChan)
		clientMu.Unlock()
	}()

	broadcastReload(ch)

	select {
	case <-clientChan:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Error("Timed out waiting for reload broadcast")
	}
}
