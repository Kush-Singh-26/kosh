package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeRequestPath(t *testing.T) {
	tests := []struct {
		name     string
		rawPath  string
		baseURL  string
		expected string
	}{
		{
			name:     "no base url",
			rawPath:  "/posts/hello.html",
			baseURL:  "",
			expected: "/posts/hello.html",
		},
		{
			name:     "base url with subdir",
			rawPath:  "/blog/posts/hello.html",
			baseURL:  "https://example.com/blog",
			expected: "/posts/hello.html",
		},
		{
			name:     "base url with subdir trailing slash",
			rawPath:  "/blog/posts/hello.html",
			baseURL:  "https://example.com/blog/",
			expected: "/posts/hello.html",
		},
		{
			name:     "raw path without slash",
			rawPath:  "posts/hello.html",
			baseURL:  "",
			expected: "/posts/hello.html",
		},
		{
			name:     "root path with base url",
			rawPath:  "/blog/",
			baseURL:  "https://example.com/blog",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRequestPath(tt.rawPath, tt.baseURL)
			if got != tt.expected {
				t.Errorf("normalizeRequestPath(%q, %q) = %q, want %q", tt.rawPath, tt.baseURL, got, tt.expected)
			}
		})
	}
}

func TestGzipHandler_RangeRequest(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("some data"))
	})

	handler := compressionHandler(next)

	req := httptest.NewRequest("GET", "/video.mp4", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-100")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("GzipHandler should NOT compress Range requests")
	}
}

func TestGzipHandler_NoGzipSupport(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("some data"))
	})

	handler := compressionHandler(next)

	req := httptest.NewRequest("GET", "/index.html", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("GzipHandler should NOT compress when Accept-Encoding does not include gzip")
	}
}

func TestIsHashedAsset(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"style.abc12345.css", true},
		{"main.deadbeef.js", true},
		{"index.html", false},
		{"style.css", false},
		{"app.123.js", false}, // too short
		{"app.longhashvalue.js", false}, // too long
		{"not.ahex.js", false},
	}

	for _, tt := range tests {
		got := isHashedAsset(tt.filename)
		if got != tt.expected {
			t.Errorf("isHashedAsset(%q) = %v, want %v", tt.filename, got, tt.expected)
		}
	}
}

func TestValidatePath_Security(t *testing.T) {
	baseDir := t.TempDir()
	
	tests := []struct {
		name     string
		userPath string
		wantErr  bool
	}{
		{"valid path", "index.html", false},
		{"valid nested path", "posts/hello.html", false},
		{"valid leading slash", "/index.html", false},
		{"valid nested leading slash", "/posts/hello.html", false},
		{"traversal attempt", "../../../etc/passwd", true},
		{"traversal with dotdot", "posts/../../etc/passwd", true},
		{"encoded traversal", "%2e%2e%2findex.html", false}, // filepath.Clean doesn't decode URL encoding
		{"root-relative safe", "/index.html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath(baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.userPath, err, tt.wantErr)
			}
		})
	}
}
