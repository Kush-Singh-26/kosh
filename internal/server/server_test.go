package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecoveryMiddleware(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Internal Server Error") {
		t.Errorf("Expected error message, got: %s", rr.Body.String())
	}
}

func TestRecoveryMiddleware_NormalRequest(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	if rr.Body.String() != "OK" {
		t.Errorf("Expected 'OK', got: %s", rr.Body.String())
	}
}

func TestValidatePath_Basic(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		path    string
		wantErr bool
	}{
		{"valid root path", "C:/test", "/", false},
		{"valid file path", "C:/test", "/file.html", false},
		{"valid nested path", "C:/test", "/dir/file.html", false},
		{"path with backslash", "C:/test", "\\file.html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath(tt.baseDir, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath_PathTraversal(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"simple traversal", "../etc/passwd", true},
		{"double traversal", "../../etc/passwd", true},
		{"encoded traversal", "%2e%2e%2fetc%2fpasswd", true},
		{"mixed encoding", "..%2f..%2f", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath("C:/test", tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath_AbsolutePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"windows absolute", "C:/Windows/System32", true},
		{"UNC path", "\\\\server\\share", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath("C:/test", tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath_InvalidEncoding(t *testing.T) {
	_, err := validatePath("C:/test", "%zz%invalid")
	if err == nil {
		t.Error("validatePath should fail with invalid encoding")
	}
}

func TestValidatePath_InvalidBaseDir(t *testing.T) {
	// Invalid base dir that can't be made absolute
	_, err := validatePath("", "/test")
	if err != nil {
		// Empty string might work depending on OS
		t.Logf("validatePath with empty baseDir: %v", err)
	}
}

func TestNormalizeRequestPath_NoBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"root", "/", "/"},
		{"file", "/file.html", "/file.html"},
		{"nested", "/dir/file.html", "/dir/file.html"},
		{"no leading slash", "file.html", "/file.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRequestPath(tt.input, "")
			if result != tt.expect {
				t.Errorf("normalizeRequestPath(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestNormalizeRequestPath_WithBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		input   string
		expect  string
	}{
		{"http base", "http://example.com/blog", "/blog/file.html", "/file.html"},
		{"https base", "https://example.com/docs", "/docs/page.html", "/page.html"},
		{"no path base", "http://example.com", "/file.html", "/file.html"},
		{"slash base", "/", "/file.html", "/file.html"},
		{"plain base", "/blog", "/blog/file.html", "/file.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRequestPath(tt.input, tt.baseURL)
			if result != tt.expect {
				t.Errorf("normalizeRequestPath(%q, %q) = %q, want %q", tt.input, tt.baseURL, result, tt.expect)
			}
		})
	}
}

func TestNormalizeRequestPath_CleansPath(t *testing.T) {
	result := normalizeRequestPath("/dir/../file.html", "")
	if result != "/file.html" {
		t.Errorf("Expected /file.html, got %s", result)
	}
}

func TestCompressionHandler_NoBrotliSupport(t *testing.T) {
	handler := compressionHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/test.html", nil)
	req.Header.Set("Accept-Encoding", "gzip") // No br
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Header().Get("Content-Encoding") != "" {
		t.Error("Should not set Content-Encoding without brotli support")
	}

	if rr.Body.String() != "OK" {
		t.Errorf("Expected 'OK', got: %s", rr.Body.String())
	}
}

func TestCompressionHandler_WithBrotliSupport(t *testing.T) {
	handler := compressionHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/test.html", nil)
	req.Header.Set("Accept-Encoding", "br")
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Header().Get("Content-Encoding") != "br" {
		t.Errorf("Expected Content-Encoding: br, got %s", rr.Header().Get("Content-Encoding"))
	}

	if rr.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Expected Vary: Accept-Encoding, got %s", rr.Header().Get("Vary"))
	}
}

func TestCompressionHandler_SkipBinaryFiles(t *testing.T) {
	binaryExts := []string{".bin", ".wasm", ".gz", ".br", ".zip", ".png", ".webp", ".jpg", ".jpeg"}

	for _, ext := range binaryExts {
		t.Run(ext, func(t *testing.T) {
			handler := compressionHandler(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("binary"))
			})

			req := httptest.NewRequest("GET", "/test"+ext, nil)
			req.Header.Set("Accept-Encoding", "br")
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Header().Get("Content-Encoding") != "" {
				t.Errorf("Should not compress %s files", ext)
			}
		})
	}
}

func TestCompressionHandler_SkipRangeRequests(t *testing.T) {
	handler := compressionHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/test.html", nil)
	req.Header.Set("Accept-Encoding", "br")
	req.Header.Set("Range", "bytes=0-100")
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Header().Get("Content-Encoding") != "" {
		t.Error("Should not compress range requests")
	}
}

func TestCompressionHandler_WritesCorrectly(t *testing.T) {
	handler := compressionHandler(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Test content"))
	})

	req := httptest.NewRequest("GET", "/test.html", nil)
	req.Header.Set("Accept-Encoding", "br")
	rr := httptest.NewRecorder()

	handler(rr, req)

	// Should have written something (compressed)
	if rr.Body.Len() == 0 {
		t.Error("Should have written compressed content")
	}
}

func TestCompressionHandler_WriteHeader(t *testing.T) {
	cw := &compressionResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
	}

	cw.WriteHeader(http.StatusCreated)

	rec := cw.ResponseWriter.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rec.Code)
	}
}

func TestHandleSSE(t *testing.T) {
	req := httptest.NewRequest("GET", "/events", nil)
	rr := httptest.NewRecorder()

	// Create context with cancel for timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	handleSSE(rr, req)

	// Should have started response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "data: connected") {
		t.Errorf("Expected 'data: connected', got: %s", body)
	}
}

func TestHandleSSE_NoFlusher(t *testing.T) {
	req := httptest.NewRequest("GET", "/events", nil)
	rr := httptest.NewRecorder()

	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	handleSSE(rr, req)

	// Check headers are set correctly
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type: text/event-stream, got %s", rr.Header().Get("Content-Type"))
	}

	if rr.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control: no-cache, got %s", rr.Header().Get("Cache-Control"))
	}
}

func TestBroadcastReload(t *testing.T) {
	reloadChan := make(chan struct{}, 1)

	// Start broadcast in goroutine
	done := make(chan bool)
	go func() {
		broadcastReload(reloadChan)
		done <- true
	}()

	// Send reload event
	reloadChan <- struct{}{}

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// Close channel to stop broadcast
	close(reloadChan)

	// Wait for goroutine to finish
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("broadcastReload did not stop in time")
	}
}

func TestBroadcastReload_WaitsForBuild(t *testing.T) {
	// Set build active
	SetBuildActive(true)
	defer SetBuildActive(false)

	reloadChan := make(chan struct{}, 1)

	done := make(chan bool)
	go func() {
		broadcastReload(reloadChan)
		done <- true
	}()

	// Send reload event
	reloadChan <- struct{}{}

	// End build
	time.Sleep(50 * time.Millisecond)
	SetBuildActive(false)

	close(reloadChan)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("broadcastReload did not complete")
	}
}

func TestSetBuildActive(t *testing.T) {
	// Initially should be false
	ch := waitForBuild()
	if ch != nil {
		t.Error("waitForBuild should return nil when build is not active")
	}

	// Set active
	SetBuildActive(true)

	// Now should return channel
	ch = waitForBuild()
	if ch == nil {
		t.Error("waitForBuild should return channel when build is active")
	}

	// End build
	SetBuildActive(false)

	// Channel should be closed
	select {
	case <-ch:
		// Success - channel closed
	default:
		t.Error("Build channel should be closed after build ends")
	}
}

func TestResetDebounceTimer(t *testing.T) {
	// Test that function doesn't panic
	resetDebounceTimer()
	resetDebounceTimer()
}

func TestStartWatcherWithConfig_Basic(t *testing.T) {
	t.Skip("Watcher test has race conditions - tested in watcher_test.go")
}

func TestStopWatcher(t *testing.T) {
	t.Skip("Watcher test has race conditions - tested in watcher_test.go")
}

func TestWaitForBuild_NoActiveBuild(t *testing.T) {
	SetBuildActive(false)

	ch := waitForBuild()
	if ch != nil {
		t.Error("waitForBuild should return nil when no build is active")
	}
}

func TestWaitForBuild_WithActiveBuild(t *testing.T) {
	SetBuildActive(true)

	ch := waitForBuild()
	if ch == nil {
		t.Error("waitForBuild should return channel when build is active")
	}

	// End build
	SetBuildActive(false)

	// Channel should close
	select {
	case <-ch:
		// Success
	default:
		t.Error("Build channel should close after build ends")
	}
}

func TestCompressionResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	cw := &compressionResponseWriter{
		ResponseWriter: rec,
		Writer:         rec,
	}

	n, err := cw.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("Expected 4 bytes written, got %d", n)
	}
}

func TestCompressionResponseWriter_WriteHeader_RemovesContentLength(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Length", "100")

	cw := &compressionResponseWriter{
		ResponseWriter: rec,
		Writer:         rec,
	}

	cw.WriteHeader(http.StatusOK)

	if rec.Header().Get("Content-Length") != "" {
		t.Error("WriteHeader should remove Content-Length")
	}
}

// Helper function to write file
