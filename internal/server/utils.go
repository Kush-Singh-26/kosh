package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	"github.com/andybalholm/brotli"
)

func validatePath(baseDir, userPath string) (string, error) {
	// URL-decode first to prevent encoded traversal sequences (%2e%2e%2f)
	decodedPath, err := url.PathUnescape(userPath)
	if err != nil {
		return "", fmt.Errorf("invalid path encoding: %w", err)
	}

	if strings.HasPrefix(decodedPath, "\\\\") {
		return "", fmt.Errorf("absolute path attempt detected")
	}

	// Reject volume-based absolute paths (e.g. C:\Windows)
	if vol := filepath.VolumeName(decodedPath); vol != "" {
		return "", fmt.Errorf("absolute path attempt detected")
	}

	// Clean the path first
	cleanUserPath := fspkg.NormalizeURLPath(decodedPath)

	// Reject if it still tries to escape via .. (path.Clean preserves leading .. if it can't resolve them)
	if strings.HasPrefix(cleanUserPath, "..") {
		return "", fmt.Errorf("path traversal attempt detected")
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
		return "", fmt.Errorf("path traversal attempt detected")
	}

	return absUserPath, nil
}

func normalizeRequestPath(rawPath, baseURL string) string {
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

func (w *compressionResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

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
		bw := brotli.NewWriterLevel(w, 4)
		defer func() { _ = bw.Close() }()
		cw := &compressionResponseWriter{Writer: bw, ResponseWriter: w}
		next(cw, r)
	}
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
	if len(hashPart) < 8 || len(hashPart) > 12 {
		return false
	}
	for _, c := range hashPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
