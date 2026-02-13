package server

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Global watcher and client management
var (
	watcher *fsnotify.Watcher
	// Broadcast channel for reload events
	reloadChan chan struct{}
	clientMu   sync.Mutex
	clients    = make(map[chan struct{}]struct{})
	watcherWg  sync.WaitGroup
)

func startWatcher(dir string) {
	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create file watcher: %v", err)
		return
	}

	// Watch the public directory recursively
	if err := watcher.Add(dir); err != nil {
		log.Printf("Failed to watch directory %s: %v", dir, err)
		return
	}

	reloadChan = make(chan struct{})

	watcherWg.Add(1)
	go func() {
		defer watcherWg.Done()
		defer watcher.Close()

		// Debounce logic
		var debounceTimer *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Ignore chmod events
				if event.Op&fsnotify.Chmod != 0 {
					continue
				}

				// Debounce rapid events (e.g. during build)
				if debounceTimer != nil {
					debounceTimer.Reset(300 * time.Millisecond)
				} else {
					debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
						select {
						case reloadChan <- struct{}{}:
						default:
							// Channel full, skip
						}
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()
}

func stopWatcher() {
	if watcher != nil {
		watcher.Close()
	}
	watcherWg.Wait()
}

// gzipResponseWriter wraps the underlying ResponseWriter to enable Gzip compression
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
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
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

// Run starts the preview server
func Run(ctx context.Context, args []string) {
	// Parse flags manually from args to avoid conflicts with main CLI flags
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "localhost", "The host/IP to bind to")
	port := fs.String("port", "2604", "The port to listen on")

	// Define flags used by the builder so they don't cause errors here
	_ = fs.Bool("drafts", false, "Include drafts (handled by builder)")
	_ = fs.String("baseurl", "", "Base URL (handled by builder)")
	_ = fs.Bool("compress", false, "Enable compression (handled by builder)")

	_ = fs.Parse(args)

	addr := fmt.Sprintf("%s:%s", *host, *port)

	// Force register the WASM mime type
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	staticDir := "./public"

	// Start the file watcher
	startWatcher(staticDir)
	defer stopWatcher()

	// Handle context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		fmt.Println("\n🛑 Shutting down server...")
		stopWatcher()
	}()

	fileServer := http.FileServer(http.Dir(staticDir))

	// --- Auto-Reload Endpoint (SSE) ---
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Set headers for Server-Sent Events
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Create a channel for this client
		clientChan := make(chan struct{})
		clientMu.Lock()
		clients[clientChan] = struct{}{}
		clientMu.Unlock()

		// Ensure cleanup on disconnect
		defer func() {
			clientMu.Lock()
			delete(clients, clientChan)
			clientMu.Unlock()
		}()

		// Initial sync check (optional, simplified)
		// Send initial event to confirm connection
		_, _ = fmt.Fprintf(w, "data: connected\n\n")
		w.(http.Flusher).Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-clientChan:
				// Reload signal received
				_, _ = fmt.Fprintf(w, "data: reload\n\n")
				w.(http.Flusher).Flush()
			}
		}
	})
	// ---------------------------------------

	// Main File Handler
	fileHandler := func(w http.ResponseWriter, r *http.Request) {
		// Normalize path early for cross-platform consistency
		rawPath := r.URL.Path
		normalizedPath := normalizeRequestPath(rawPath)

		// Validate path to prevent traversal attacks
		fullPath, err := validatePath(staticDir, normalizedPath)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("403 - Forbidden: Invalid path"))
			return
		}

		// Check if file exists
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
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

		// Add cache headers for hashed assets
		filename := filepath.Base(normalizedPath)
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
	}

	http.HandleFunc("/", gzipHandler(fileHandler))

	// Start reload broadcaster
	go func() {
		for range reloadChan {
			clientMu.Lock()
			for clientChan := range clients {
				select {
				case clientChan <- struct{}{}:
				default:
					// Client buffer full, skip
				}
			}
			clientMu.Unlock()
		}
	}()

	// Create HTTP server with shutdown support
	httpServer := &http.Server{
		Addr:    addr,
		Handler: nil, // uses DefaultServeMux
	}

	// Shutdown handler - watches for context cancellation
	go func() {
		<-ctx.Done()
		fmt.Println("\n🛑 Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	fmt.Printf("🌍 Serving on http://%s\n", addr)
	if *host == "0.0.0.0" {
		fmt.Println("   (Accessible on your local network)")
	}
	fmt.Println("   (Auto-reload enabled via /events)")

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	fmt.Println("✅ Server stopped.")
}

// isHashedAsset checks if filename contains a content hash (e.g., layout.a1b2c3.css)
func isHashedAsset(filename string) bool {
	// Check for pattern: name.8-12chars.hash.ext
	// Examples: layout.a1b2c3d4.css, main.1234567890ab.js
	parts := strings.Split(filename, ".")
	if len(parts) >= 3 {
		// Middle part should be 8-12 hex characters
		hashPart := parts[len(parts)-2]
		if len(hashPart) >= 8 && len(hashPart) <= 12 {
			// Check if it looks like a hex hash
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
