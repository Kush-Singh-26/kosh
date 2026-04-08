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

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/ui"
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

// WriteHeader records the status and delegates to the underlying writer.
func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush forwards the flush to the underlying writer when supported.
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
			orchestration.HTTPLog(r.Method, r.URL.Path, sw.status, duration)
		}
	})
}

// ServerOptions configures the development server.
type ServerOptions struct {
	Ctx         context.Context
	Args        []string
	OutputDir   string
	BaseURL     string
	BuildConfig *config.BuildConfig
	Reporter    ui.Reporter
}

// Run starts the development HTTP server.
func Run(opts ServerOptions) {
	ctx := opts.Ctx
	args := opts.Args
	outputDir := opts.OutputDir
	baseURL := opts.BaseURL
	buildCfg := opts.BuildConfig
	reporter := opts.Reporter

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

	async.FireAndForget(ctx, slog.Default(), "server watcher shutdown", func() error {
		<-ctx.Done()
		orchestration.DevLogInfo("Shutting down server...")
		stopWatcher()
		return nil
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/events", handleSSE)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !waitForBuildCompletion(w, r) {
			return
		}

		rawPath := r.URL.Path
		normalizedPath := normalizeRequestPath(rawPath, baseURL)
		r.URL.Path = normalizedPath

		fullPath, err := validatePath(staticDir, normalizedPath)
		if err != nil {
			renderError(w, http.StatusForbidden, "403 - Forbidden: Invalid path")
			return
		}

		handleFileRequest(fileRequestOptions{
			writer:         w,
			request:        r,
			staticDir:      staticDir,
			fullPath:       fullPath,
			normalizedPath: normalizedPath,
		})
	})

	if reloadEvents != nil {
		async.FireAndForget(ctx, slog.Default(), "reload broadcast", func() error {
			broadcastReload(reloadEvents)
			return nil
		})
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: loggingMiddleware(recoveryMiddleware(mux)),
	}

	async.FireAndForget(ctx, slog.Default(), "http server shutdown", func() error {
		<-ctx.Done()
		orchestration.DevLogInfo("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
		return nil
	})

	if reporter != nil {
		reporter.Status("Live Preview: http://" + addr)
	} else {
		orchestration.DevLogInfo("Serving on http://" + addr)
	}

	if *host == "0.0.0.0" {
		orchestration.DevLogInfo("Accessible on your local network")
	}

	orchestration.DevLogInfo("Auto-reload enabled via /events")

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
	orchestration.DevLogSuccess("Server stopped")
}

func waitForBuildCompletion(w http.ResponseWriter, r *http.Request) bool {
	ch := waitForBuild()
	if ch == nil {
		return true
	}

	select {
	case <-ch:
		return true
	case <-r.Context().Done():
		return false
	case <-time.After(5 * time.Second):
		slog.Warn("Wait for build timed out", "path", r.URL.Path)
		return true
	}
}

type fileRequestOptions struct {
	writer         http.ResponseWriter
	request        *http.Request
	staticDir      string
	fullPath       string
	normalizedPath string
}

type responseHeaderOptions struct {
	writer         http.ResponseWriter
	fullPath       string
	normalizedPath string
	fileInfo       os.FileInfo
	preCompressed  bool
}

func handleFileRequest(opts fileRequestOptions) {
	acceptEncoding := opts.request.Header.Get("Accept-Encoding")
	ext := strings.ToLower(filepath.Ext(opts.normalizedPath))
	preCompressed := false

	if ext != ".wasm" && ext != ".bin" && strings.Contains(acceptEncoding, "br") {
		if _, err := os.Stat(opts.fullPath + ".br"); err == nil {
			opts.fullPath += ".br"
			preCompressed = true
		}
	}

	serve := func(w http.ResponseWriter, r *http.Request) {
		fileInfo, err := os.Stat(opts.fullPath)
		if err != nil {
			handleFileError(w, opts.staticDir, err)
			return
		}

		if fileInfo.IsDir() {
			if handleDirectory(w, r, opts.fullPath) {
				return
			}
		}

		setResponseHeaders(responseHeaderOptions{
			writer:         w,
			fullPath:       opts.fullPath,
			normalizedPath: opts.normalizedPath,
			fileInfo:       fileInfo,
			preCompressed:  preCompressed,
		})
		http.ServeFile(w, r, opts.fullPath)
	}

	if preCompressed {
		serve(opts.writer, opts.request)
	} else {
		compressionHandler(serve)(opts.writer, opts.request)
	}
}

func handleFileError(w http.ResponseWriter, staticDir string, err error) {
	if os.IsNotExist(err) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
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
}

func handleDirectory(w http.ResponseWriter, r *http.Request, fullPath string) bool {
	indexPath := filepath.Join(fullPath, "index.html")
	if indexInfo, err := os.Stat(indexPath); err == nil && !indexInfo.IsDir() {
		http.ServeFile(w, r, indexPath)
		return true
	}
	return false
}

func setResponseHeaders(opts responseHeaderOptions) {
	filename := filepath.Base(opts.normalizedPath)

	// Set Cache-Control
	if isHashedAsset(filename) {
		opts.writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if opts.fileInfo.IsDir() || strings.HasSuffix(filename, ".html") || strings.HasSuffix(filename, ".wasm") || strings.HasSuffix(filename, ".bin") {
		opts.writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		opts.writer.Header().Set("Pragma", "no-cache")
		opts.writer.Header().Set("Expires", "0")
	} else {
		opts.writer.Header().Set("Cache-Control", "public, max-age=60")
	}

	if opts.preCompressed {
		opts.writer.Header().Set("Content-Encoding", "br")
		opts.writer.Header().Set("Vary", "Accept-Encoding")
		originalExt := strings.ToLower(filepath.Ext(opts.normalizedPath))
		if contentType := mime.TypeByExtension(originalExt); contentType != "" {
			opts.writer.Header().Set("Content-Type", contentType)
		}
	} else {
		ext := strings.ToLower(filepath.Ext(opts.fullPath))
		switch ext {
		case ".css":
			opts.writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js":
			opts.writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}
	}
}

func renderError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
