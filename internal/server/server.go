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
		slog.Info("\n🛑 Shutting down server...")
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
		} else if fileInfo.IsDir() || strings.HasSuffix(filename, ".html") {
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
		Handler: recoveryMiddleware(mux),
	}

	go func() {
		<-ctx.Done()
		slog.Info("\n🛑 Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info(fmt.Sprintf("🌍 Serving on http://%s", addr))
	if *host == "0.0.0.0" {
		slog.Info("   (Accessible on your local network)")
	}
	slog.Info("   (Auto-reload enabled via /events)")

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
	slog.Info("✅ Server stopped.")
}
