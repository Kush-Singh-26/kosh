package server

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

func validatePath(baseDir, userPath string) (string, error) {
	// Reject actual absolute paths (e.g. C:\Windows or /etc/passwd)
	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("absolute path attempt detected")
	}

	// Clean the path first
	cleanUserPath := filepath.Clean(userPath)

	// Reject if it still tries to escape via .. (filepath.Clean preserves leading .. if it can't resolve them)
	if strings.HasPrefix(cleanUserPath, "..") {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	// Trim leading slashes for joining
	trimmedPath := strings.TrimLeft(cleanUserPath, "/\\")

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	// Join and get absolute path
	absUserPath, err := filepath.Abs(filepath.Join(absBase, trimmedPath))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the resulting path is within the base directory
	if !strings.HasPrefix(absUserPath, absBase) {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	return absUserPath, nil
}

func normalizeRequestPath(rawPath, baseURL string) string {
	prefix := ""
	if baseURL != "" {
		if strings.HasPrefix(baseURL, "http://") {
			parts := strings.SplitN(strings.TrimPrefix(baseURL, "http://"), "/", 2)
			if len(parts) > 1 {
				prefix = "/" + parts[1]
			}
		} else if strings.HasPrefix(baseURL, "https://") {
			parts := strings.SplitN(strings.TrimPrefix(baseURL, "https://"), "/", 2)
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
	return filepath.ToSlash(filepath.Clean(rawPath))
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
		// Do not compress if the client doesn't support it, or if it's a range request
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			next(w, r)
			return
		}

		// Skip compression for already compressed formats or binary files
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if ext == ".bin" || ext == ".wasm" || ext == ".gz" || ext == ".br" || ext == ".zip" || ext == ".png" || ext == ".webp" || ext == ".jpg" || ext == ".jpeg" {
			next(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		cw := &compressionResponseWriter{Writer: gz, ResponseWriter: w}
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
