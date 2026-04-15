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
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

const (
	defaultStatusCode       = http.StatusOK
	slowRequestThreshold    = 500 * time.Millisecond
	defaultShutdownTimeout  = 5 * time.Second
	defaultDebounceDuration = 500 * time.Millisecond
	buildWaitTimeout        = 5 * time.Second
	cacheMaxAgeHashed       = 31536000
	cacheMaxAgeDefault      = 60
)

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("Server panic recovered", "error", err, "path", request.URL.Path)
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = writer.Write([]byte("500 - Internal Server Error"))
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status and delegates to the underlying writer.
func (writer *statusWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

// Flush forwards the flush to the underlying writer when supported.
func (writer *statusWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		swr := &statusWriter{ResponseWriter: writer, status: defaultStatusCode}
		next.ServeHTTP(swr, request)
		duration := time.Since(start)
		// Skip logging for SSE /events endpoint - not useful to log heartbeats
		if request.URL.Path != "/events" && (swr.status >= http.StatusBadRequest || duration > slowRequestThreshold) {
			orchestration.HTTPLog(request.Method, request.URL.Path, swr.status, duration)
		}
	})
}

// Options configures the development server.
type Options struct {
	Ctx           context.Context
	Args          []string
	OutputDir     string
	RootDirectory string
	SiteRoot      string
	BaseURL       string
	BuildConfig   *config.BuildConfig
	Reporter      ui.Reporter
	IsDev         bool
}

type serveConfig struct {
	addr             string
	host             string
	staticDir        string
	rootDirectory    string
	shutdownTimeout  time.Duration
	debounceDuration time.Duration
	isDev            bool
}

func parseServeFlags(args []string) (string, string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "localhost", "The host/IP to bind to")
	port := fs.String("port", "2604", "The port to listen on")

	_ = fs.Bool("drafts", false, "Include drafts (handled by builder)")
	_ = fs.String("baseurl", "", "Base URL (handled by builder)")
	_ = fs.Bool("compress", false, "Enable compression (handled by builder)")

	_ = fs.Parse(args)
	return *host, *port
}

func resolveStaticDir(outputDir string) string {
	if outputDir == "" {
		return "./public"
	}
	return outputDir
}

func resolveShutdownTimeout(buildCfg *config.BuildConfig) time.Duration {
	if buildCfg == nil {
		return defaultShutdownTimeout
	}
	return buildCfg.ShutdownTimeout
}

func resolveDebounceDuration(buildCfg *config.BuildConfig) time.Duration {
	if buildCfg == nil {
		return defaultDebounceDuration
	}
	return buildCfg.DebounceDuration
}

func startWatcher(ctx context.Context, watchConfig watchConfig) chan string {
	reloadEvents := startWatcherWithConfig(watchConfig)
	async.FireAndForget(ctx, slog.Default(), "server watcher shutdown", func() error {
		<-ctx.Done()
		orchestration.DevLogInfo("Shutting down server...")
		stopWatcher()
		return nil
	})
	return reloadEvents
}

func startReloadBroadcast(ctx context.Context, reloadEvents chan string) {
	if reloadEvents == nil {
		return
	}
	async.FireAndForget(ctx, slog.Default(), "reload broadcast", func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case msg, ok := <-reloadEvents:
				if !ok {
					return nil
				}
				parts := strings.SplitN(msg, ":", 2)
				if len(parts) == 2 {
					BroadcastReload(parts[0], parts[1])
				} else {
					BroadcastReload(msg, "")
				}
			}
		}
	})
}

func buildServeConfig(opts Options, host, port string) serveConfig {
	return serveConfig{
		addr:             fmt.Sprintf("%s:%s", host, port),
		host:             host,
		staticDir:        resolveStaticDir(opts.OutputDir),
		rootDirectory:    opts.RootDirectory,
		shutdownTimeout:  resolveShutdownTimeout(opts.BuildConfig),
		debounceDuration: resolveDebounceDuration(opts.BuildConfig),
		isDev:            opts.IsDev,
	}
}

func registerServerShutdown(ctx context.Context, httpServer *http.Server, timeout time.Duration) {
	async.FireAndForget(ctx, slog.Default(), "http server shutdown", func() error {
		<-ctx.Done()
		orchestration.DevLogInfo("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
		return nil
	})
}

func logServeStatus(reporter ui.Reporter, addr, host string) {
	if reporter != nil {
		reporter.Status("Live Preview: http://" + addr)
	} else {
		orchestration.DevLogInfo("Serving on http://" + addr)
	}
	if host == "0.0.0.0" {
		orchestration.DevLogInfo("Accessible on your local network")
	}
	orchestration.DevLogInfo("Auto-reload enabled via /events")
}

// Run starts the development HTTP server.
func Run(opts Options) {
	ctx := opts.Ctx
	args := opts.Args
	baseURL := opts.BaseURL
	reporter := opts.Reporter

	host, port := parseServeFlags(args)
	cfg := buildServeConfig(opts, host, port)

	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".bin", "application/octet-stream")

	assetsDir := "assets"
	if cfg.rootDirectory != "" {
		assetsDir = filepath.Join(cfg.rootDirectory, "assets")
	}

	var exclusions []string
	if cfg.rootDirectory != "" {
		// Exclude blog output directory from root watch to avoid infinite reload loops
		exclusions = append(exclusions, cfg.staticDir)
		if opts.SiteRoot != "" {
			// Exclude the entire Kosh project root from the portfolio's watcher
			exclusions = append(exclusions, opts.SiteRoot)
		}
		// Exclude noisy VCS and dependency directories
		exclusions = append(exclusions, filepath.Join(cfg.rootDirectory, ".git"))
		exclusions = append(exclusions, filepath.Join(cfg.rootDirectory, "node_modules"))
	}

	dirs := make(map[string]string)
	dirs[cfg.staticDir] = "site"
	if cfg.rootDirectory != "" {
		dirs[cfg.rootDirectory] = "root"
	}
	if _, err := os.Stat(assetsDir); err == nil {
		dirs[assetsDir] = "all"
	}

	reloadEvents := startWatcher(ctx, watchConfig{
		Dirs:       dirs,
		Debounce:   cfg.debounceDuration,
		Exclusions: exclusions,
	})
	defer stopWatcher()

	mux := http.NewServeMux()

	mux.HandleFunc("/events", handleSSE)

	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if !waitForBuildCompletion(writer, request) {
			return
		}

		rawPath := request.URL.Path
		contentPrefix := GetBaseURLPrefix(baseURL)
		effectiveStaticDir := cfg.staticDir
		var normalizedPath string

		switch {
		case strings.HasPrefix(rawPath, "/static/"):
			// Always prioritize consolidated site assets for /static/ requests
			normalizedPath = normalizeRequestPath(rawPath, baseURL)
		case HasPathPrefix(rawPath, contentPrefix):
			normalizedPath = normalizeRequestPath(rawPath, baseURL)
		case cfg.rootDirectory != "":
			effectiveStaticDir = cfg.rootDirectory
			normalizedPath = rawPath // Already starts with /
		default:
			normalizedPath = normalizeRequestPath(rawPath, baseURL)
		}
		//nolint:gocritic

		request.URL.Path = normalizedPath

		fullPath, err := validatePath(effectiveStaticDir, normalizedPath)
		if err != nil {
			slog.Error("Routing failed - invalid path",
				"rawPath", rawPath,
				"contentPrefix", contentPrefix,
				"effectiveStaticDir", effectiveStaticDir,
				"normalizedPath", normalizedPath,
				"error", err)
			http.Error(writer, "Forbidden", http.StatusForbidden)
			return
		}

		slog.Debug("Routing request",
			"rawPath", rawPath,
			"contentPrefix", contentPrefix,
			"rootDirectory", cfg.rootDirectory,
			"effectiveStaticDir", effectiveStaticDir,
			"normalizedPath", normalizedPath,
			"fullPath", fullPath)

		handleFileRequest(fileRequestOptions{
			writer:          writer,
			request:         request,
			staticDir:       effectiveStaticDir,
			engineOutputDir: cfg.staticDir,
			fullPath:        fullPath,
			normalizedPath:  normalizedPath,
			isDev:           cfg.isDev,
		})
	})

	startReloadBroadcast(ctx, reloadEvents)

	httpServer := &http.Server{
		Addr:           cfg.addr,
		Handler:        loggingMiddleware(recoveryMiddleware(mux)),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	registerServerShutdown(ctx, httpServer, cfg.shutdownTimeout)

	logServeStatus(reporter, cfg.addr, cfg.host)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		stopWatcher()
		os.Exit(1) //nolint:gocritic
	}
	orchestration.DevLogSuccess("Server stopped")
}

func waitForBuildCompletion(_ http.ResponseWriter, request *http.Request) bool {
	buildDone := waitForBuild()
	if buildDone == nil {
		return true
	}

	select {
	case <-buildDone:
		return true
	case <-request.Context().Done():
		return false
	case <-time.After(buildWaitTimeout):
		slog.Warn("Wait for build timed out", "path", request.URL.Path)
		return true
	}
}

type fileRequestOptions struct {
	writer          http.ResponseWriter
	request         *http.Request
	staticDir       string // Directory being served for this request
	engineOutputDir string // The SSG's output directory
	fullPath        string
	normalizedPath  string
	isDev           bool
}

type responseHeaderOptions struct {
	writer         http.ResponseWriter
	fullPath       string
	normalizedPath string
	fileInfo       os.FileInfo
	preCompressed  bool
	isDev          bool
}

func handleFileRequest(opts fileRequestOptions) {
	acceptEncoding := opts.request.Header.Get("Accept-Encoding")
	ext := strings.ToLower(filepath.Ext(opts.normalizedPath))
	preCompressed := false

	if ext != ".bin" && strings.Contains(acceptEncoding, "br") {
		if _, err := os.Stat(opts.fullPath + ".br"); err == nil {
			opts.fullPath += ".br"
			preCompressed = true
		}
	}

	serve := func(writer http.ResponseWriter, request *http.Request) {
		fileInfo, err := os.Stat(opts.fullPath)
		if err != nil {
			handleFileError(writer, opts.staticDir, err)
			return
		}

		if fileInfo.IsDir() {
			indexPath := filepath.Join(opts.fullPath, "index.html")
			if indexInfo, err := os.Stat(indexPath); err == nil && !indexInfo.IsDir() {
				opts.fullPath = indexPath
				fileInfo = indexInfo
			} else {
				handleFileError(writer, opts.staticDir, os.ErrNotExist)
				return
			}
		}

		// Inject live-reload script if serving HTML from rootDirectory (not the SSG output)
		isRootHTML := strings.HasSuffix(strings.ToLower(opts.fullPath), ".html") &&
			opts.staticDir != "" &&
			fspkg.NormalizePath(opts.staticDir) != fspkg.NormalizePath(opts.engineOutputDir)

		if isRootHTML {
			slog.Debug("Injecting live-reload script into root HTML", "path", opts.normalizedPath)
			data, err := os.ReadFile(opts.fullPath)
			if err == nil {
				html := InjectLiveReload(string(data))
				setResponseHeaders(responseHeaderOptions{
					writer:         writer,
					fullPath:       opts.fullPath,
					normalizedPath: opts.normalizedPath,
					fileInfo:       fileInfo,
					preCompressed:  false,
				})
				_, _ = writer.Write([]byte(html))
				return
			}
		}

		setResponseHeaders(responseHeaderOptions{
			writer:         writer,
			fullPath:       opts.fullPath,
			normalizedPath: opts.normalizedPath,
			fileInfo:       fileInfo,
			preCompressed:  preCompressed,
			isDev:          opts.isDev,
		})
		http.ServeFile(writer, request, opts.fullPath)
	}

	if preCompressed {
		serve(opts.writer, opts.request)
	} else {
		compressionHandler(serve)(opts.writer, opts.request)
	}
}

func handleFileError(writer http.ResponseWriter, staticDir string, err error) {
	if os.IsNotExist(err) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		writer.Header().Set("Pragma", "no-cache")
		writer.Header().Set("Expires", "0")
		writer.WriteHeader(http.StatusNotFound)
		notFoundPath := filepath.Join(staticDir, "404.html")
		if content, readErr := os.ReadFile(notFoundPath); readErr == nil {
			_, _ = writer.Write([]byte(InjectLiveReload(string(content))))
		} else {
			_, _ = writer.Write([]byte(InjectLiveReload("404 - Page Not Found")))
		}
	} else {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("500 - Internal Server Error"))
	}
}

func setResponseHeaders(opts responseHeaderOptions) {
	filename := filepath.Base(opts.normalizedPath)

	// Set Cache-Control
	switch {
	case isHashedAsset(filename):
		opts.writer.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", cacheMaxAgeHashed))
	case opts.isDev, opts.fileInfo.IsDir(), strings.HasSuffix(filename, ".html"), strings.HasSuffix(filename, ".wasm"), strings.HasSuffix(filename, ".bin"):
		opts.writer.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		opts.writer.Header().Set("Pragma", "no-cache")
		opts.writer.Header().Set("Expires", "0")
	default:
		opts.writer.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cacheMaxAgeDefault))
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
