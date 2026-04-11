package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/andybalholm/brotli"
)

const (
	brotliLevel          = 4
	hashedAssetMinLength = 8
	hashedAssetMaxLength = 12
)

const liveReloadScript = `
<script>
(function() {
    const hosts = ["localhost", "127.0.0.1", "0.0.0.0"];
    if (hosts.includes(window.location.hostname)) {
        if ('serviceWorker' in navigator) {
            navigator.serviceWorker.getRegistrations().then(regs => {
                regs.forEach(r => r.unregister());
            });
        }
        if ('caches' in window) {
            caches.keys().then(names => {
                names.forEach(n => caches.delete(n));
            });
        }
        let reloadTimeout;
        const source = new EventSource("/events");
        source.onopen = function() { console.log("🚀 Kosh Live Reload connected"); };
        source.onmessage = function(e) {
            if (e.data === "site" || e.data === "root" || e.data === "all" || e.data === "reload") {
                console.log("♻️ Change detected: " + e.data);
                if (reloadTimeout) clearTimeout(reloadTimeout);
                reloadTimeout = setTimeout(() => {
                    window.location.reload(true);
                }, 250);
            }
        };
        source.onerror = function() { source.close(); };
    }
})();
</script>
`

// InjectLiveReload injects the live-reload script before the </body> tag or at the end of the HTML.
func InjectLiveReload(html string) string {
	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", liveReloadScript+"</body>", 1)
	}
	return html + liveReloadScript
}

var (
	errAbsolutePath  = errors.New("absolute path attempt detected")
	errPathTraversal = errors.New("path traversal attempt detected")
)

func validatePath(baseDir, userPath string) (string, error) {
	// URL-decode first to prevent encoded traversal sequences (%2e%2e%2f)
	decodedPath, err := url.PathUnescape(userPath)
	if err != nil {
		return "", fmt.Errorf("invalid path encoding: %w", err)
	}

	// Reject UNC paths (\\server\share)
	if strings.HasPrefix(decodedPath, "\\\\") {
		return "", errAbsolutePath
	}

	// Reject volume-based absolute paths (e.g. C:\Windows on Windows)
	if vol := filepath.VolumeName(decodedPath); vol != "" {
		return "", errAbsolutePath
	}

	// Cross-platform check: reject paths that look like Windows absolute paths even on Linux
	// Pattern: letter:/path (e.g., C:/Windows/System32)
	if len(decodedPath) >= 2 && decodedPath[1] == ':' && ((decodedPath[0] >= 'a' && decodedPath[0] <= 'z') || (decodedPath[0] >= 'A' && decodedPath[0] <= 'Z')) {
		return "", errAbsolutePath
	}

	// Clean the path first
	cleanUserPath := fspkg.NormalizeURLPath(decodedPath)

	// Reject if it still tries to escape via .. (path.Clean preserves leading .. if it can't resolve them)
	if strings.HasPrefix(cleanUserPath, "..") {
		return "", errPathTraversal
	}

	// Trim leading slashes for joining
	trimmedPath := strings.TrimLeft(cleanUserPath, "/")

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	// Join and get absolute path
	absUserPath, err := filepath.Abs(filepath.Join(absBase, filepath.FromSlash(trimmedPath)))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the resulting path is within the base directory
	if absUserPath != absBase && !strings.HasPrefix(absUserPath, absBase+string(filepath.Separator)) {
		return "", errPathTraversal
	}

	return absUserPath, nil
}

// GetBaseURLPrefix extracts the path prefix from the baseURL (e.g., "/blogs" from "https://example.com/blogs").
func GetBaseURLPrefix(baseURL string) string {
	prefix := ""
	if baseURL != "" {
		if after, ok := strings.CutPrefix(baseURL, "http://"); ok {
			parts := strings.SplitN(after, "/", 2)
			if len(parts) > 1 {
				prefix = "/" + parts[1]
			}
		} else if after, ok := strings.CutPrefix(baseURL, "https://"); ok {
			parts := strings.SplitN(after, "/", 2)
			if len(parts) > 1 {
				prefix = "/" + parts[1]
			}
		} else {
			prefix = baseURL
		}
	}

	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

func normalizeRequestPath(rawPath, baseURL string) string {
	prefix := GetBaseURLPrefix(baseURL)

	if prefix != "" && prefix != "/" {
		rawPath = strings.TrimPrefix(rawPath, prefix)
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	rawPath = fspkg.NormalizeURLPath(rawPath)
	if rawPath == "." {
		rawPath = "/"
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	return rawPath
}

type compressionResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

// Write writes through the compression writer.
func (w *compressionResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// WriteHeader clears Content-Length and writes the header.
func (w *compressionResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func compressionHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Do not compress if the client doesn't support it (Brotli), or if it's a range request
		// Also skip compression for SSE endpoints
		path := r.URL.Path
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "br") || r.Header.Get("Range") != "" || path == "/events" {
			next(w, r)
			return
		}

		// Skip compression for already compressed formats or binary files
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if ext == ".bin" || ext == ".wasm" || ext == ".gz" || ext == ".br" || ext == ".zip" || ext == ".png" || ext == ".webp" || ext == ".jpg" || ext == ".jpeg" {
			next(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Vary", "Accept-Encoding")
		bw := brotli.NewWriterLevel(w, brotliLevel)
		defer func() { _ = bw.Close() }()
		cw := &compressionResponseWriter{Writer: bw, ResponseWriter: w}
		next(cw, r)
	}
}

// HasPathPrefix reports whether the path starts with the prefix as a full path segment.
// For example, if prefix is "/blogs", it matches "/blogs" and "/blogs/",
// but not "/blogs-src".
func HasPathPrefix(path, prefix string) bool {
	if prefix == "" {
		return false
	}
	if path == prefix {
		return true
	}
	if strings.HasPrefix(path, prefix) {
		if len(path) > len(prefix) {
			lastChar := path[len(prefix)]
			return lastChar == '/'
		}
		return true
	}
	return false
}

func isHashedAsset(filename string) bool {
	// Find the last dot (extension separator)
	lastDot := strings.LastIndexByte(filename, '.')
	if lastDot <= 0 {
		return false
	}
	// Find the second-to-last dot (hash separator)
	hashEnd := lastDot
	prevDot := strings.LastIndexByte(filename[:hashEnd], '.')
	if prevDot < 0 {
		return false
	}
	hashPart := filename[prevDot+1 : hashEnd]
	// esbuild uses alphanumeric hashes of length 8
	if len(hashPart) < hashedAssetMinLength || len(hashPart) > hashedAssetMaxLength {
		return false
	}
	for _, c := range hashPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
