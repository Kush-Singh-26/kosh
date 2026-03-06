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
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	cleanPath := filepath.Clean(userPath)

	absUserPath, err := filepath.Abs(filepath.Join(baseDir, cleanPath))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	if !strings.HasPrefix(absUserPath, absBase) {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	relPath, err := filepath.Rel(absBase, absUserPath)
	if err != nil {
		return "", fmt.Errorf("path validation error: %w", err)
	}

	if strings.Contains(relPath, "..") {
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

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func gzipHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Do not gzip if the client doesn't support it, or if it's a range request
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			next(w, r)
			return
		}

		// Skip compression for already compressed formats, large binary files, or the search index
		// search.bin is internally gzipped and handled by WASM DecompressionStream
		// search.wasm is large and gzipping it on every request is CPU intensive
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if ext == ".bin" || ext == ".wasm" || ext == ".gz" || ext == ".zip" || ext == ".png" || ext == ".webp" || ext == ".jpg" || ext == ".jpeg" {
			next(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next(gzw, r)
	}
}

func isHashedAsset(filename string) bool {
	parts := strings.Split(filename, ".")
	if len(parts) >= 3 {
		hashPart := parts[len(parts)-2]
		if len(hashPart) >= 8 && len(hashPart) <= 12 {
			isHex := true
			for _, c := range hashPart {
				if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
					isHex = false
					break
				}
			}
			return isHex
		}
	}
	return false
}
