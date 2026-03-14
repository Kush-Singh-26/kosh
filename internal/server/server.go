package server

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/run"
)

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("Server panic recovered", "error", err, "path", r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("500 - Internal Server Error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)
		// Skip logging for SSE /events endpoint - not useful to log heartbeats
		if r.URL.Path != "/events" && (sw.status >= 400 || duration > 500*time.Millisecond) {
			run.HTTPLog(r.Method, r.URL.Path, sw.status, duration)
		}
	})
}

func Run(ctx context.Context, args []string, outputDir string, baseURL string, buildCfg *config.BuildConfig) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "localhost", "The host/IP to bind to")
	port := fs.String("port", "2604", "The port to listen on")

	_ = fs.Bool("drafts", false, "Include drafts (handled by builder)")
	_ = fs.String("baseurl", "", "Base URL (handled by builder)")
	_ = fs.Bool("compress", false, "Enable compression (handled by builder)")

	_ = fs.Parse(args)

	addr := fmt.Sprintf("%s:%s", *host, *port)

	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".bin", "application/octet-stream")

	staticDir := outputDir
	if staticDir == "" {
		staticDir = "./public"
	}

	// Get shutdown timeout from build config
	shutdownTimeout := 5 * time.Second
	if buildCfg != nil {
		shutdownTimeout = buildCfg.ShutdownTimeout
	}

	debounceDuration := 500 * time.Millisecond
	if buildCfg != nil {
		debounceDuration = buildCfg.DebounceDuration
	}

	reloadEvents := startWatcherWithConfig(staticDir, debounceDuration)
	defer stopWatcher()

	go func() {
		<-ctx.Done()
		run.DevLogInfo("Shutting down server...")
		stopWatcher()
	}()

	fileServer := http.FileServer(http.Dir(staticDir))
	mux := http.NewServeMux()

	mux.HandleFunc("/events", handleSSE)

	mux.HandleFunc("/", compressionHandler(func(w http.ResponseWriter, r *http.Request) {
		// If build is active, wait for it to complete or request cancellation
		if ch := waitForBuild(); ch != nil {
			select {
			case <-ch:
				// Build finished, proceed
			case <-r.Context().Done():
				// Request cancelled (tab closed), exit immediately
				return
			case <-time.After(5 * time.Second):
				slog.Warn("Wait for build timed out", "path", r.URL.Path)
			}
		}

		rawPath := r.URL.Path
		normalizedPath := normalizeRequestPath(rawPath, baseURL)

		// Update request path for fileServer to handle baseURL prefix
		r.URL.Path = normalizedPath

		fullPath, err := validatePath(staticDir, normalizedPath)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("403 - Forbidden: Invalid path"))
			return
		}

		// Handle pre-compressed files.
		// Skip WASM and binary files: WebAssembly.instantiateStreaming requires
		// raw bytes — setting Content-Encoding on a WASM response causes the
		// browser's Fetch decompression pipeline to abort with
		// "Response body loading was aborted".
		acceptEncoding := r.Header.Get("Accept-Encoding")
		ext := strings.ToLower(filepath.Ext(normalizedPath))
		if ext != ".wasm" && ext != ".bin" {
			if strings.Contains(acceptEncoding, "br") {
				if _, err := os.Stat(fullPath + ".br"); err == nil {
					w.Header().Set("Content-Encoding", "br")
					w.Header().Set("Vary", "Accept-Encoding")
					fullPath += ".br"
				}
			}
		}

		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Debug("File not found", "fullPath", fullPath, "normalizedPath", normalizedPath)
				w.WriteHeader(http.StatusNotFound)
				notFoundPath := filepath.Join(staticDir, "404.html")
				if content, readErr := os.ReadFile(notFoundPath); readErr == nil {
					_, _ = w.Write(content)
				} else {
					_, _ = w.Write([]byte("404 - Page Not Found"))
				}
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("500 - Internal Server Error"))
			}
			return
		}

		filename := filepath.Base(normalizedPath)

		// Special handling for directory requests - serve index.html directly
		// This prevents Go's http.FileServer from redirecting /tags/ to tags/
		if fileInfo.IsDir() {
			indexPath := filepath.Join(fullPath, "index.html")
			if indexInfo, err := os.Stat(indexPath); err == nil && !indexInfo.IsDir() {
				// Serve index.html directly without redirect
				http.ServeFile(w, r, indexPath)
				return
			}
		}

		if isHashedAsset(filename) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if fileInfo.IsDir() || strings.HasSuffix(filename, ".html") || strings.HasSuffix(filename, ".wasm") || strings.HasSuffix(filename, ".bin") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=60")
		}

		fileServer.ServeHTTP(w, r)
	}))

	if reloadEvents != nil {
		go broadcastReload(reloadEvents)
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: loggingMiddleware(recoveryMiddleware(mux)),
	}

	go func() {
		<-ctx.Done()
		run.DevLogInfo("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	run.DevLogInfo("Serving on http://" + addr)
	if *host == "0.0.0.0" {
		run.DevLogInfo("Accessible on your local network")
	}

	run.DevLogInfo("Auto-reload enabled via /events")

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
	run.DevLogSuccess("Server stopped")
}
