package server

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// gzipResponseWriter wraps the underlying ResponseWriter to enable Gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
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
func Run(args []string) {
	// Parse flags manually from args to avoid conflicts with main CLI flags
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "localhost", "The host/IP to bind to")
	port := fs.String("port", "2604", "The port to listen on")

	_ = fs.Parse(args)

	addr := fmt.Sprintf("%s:%s", *host, *port)

	// Force register the WASM mime type
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	staticDir := "./public"
	fileServer := http.FileServer(http.Dir(staticDir))

	// --- Auto-Reload Endpoint (SSE) ---
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Set headers for Server-Sent Events
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Check for file changes every 500ms
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var lastMod time.Time
		// Initialize with current state
		if t, err := getLatestModTime(staticDir); err == nil {
			lastMod = t
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				currentMod, err := getLatestModTime(staticDir)
				if err != nil {
					continue
				}
				// If files have been modified since we last checked
				if currentMod.After(lastMod) {
					lastMod = currentMod
					// Send reload signal
					_, _ = fmt.Fprintf(w, "data: reload\n\n")
					w.(http.Flusher).Flush()
				}
			}
		}
	})
	// ---------------------------------------

	// Main File Handler
	fileHandler := func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(r.URL.Path)
		// If path starts with /blogs/, strip it for local development
		if strings.HasPrefix(path, "/blogs/") {
			path = strings.TrimPrefix(path, "/blogs/")
		} else if strings.HasPrefix(path, "\\blogs\\") {
			path = strings.TrimPrefix(path, "\\blogs\\")
		}

		fullPath := filepath.Join(staticDir, path)

		// Check if file exists
		_, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			notFoundPath := filepath.Join(staticDir, "404.html")
			content, readErr := os.ReadFile(notFoundPath)
			if readErr != nil {
				_, _ = w.Write([]byte("404 - Page Not Found"))
			} else {
				_, _ = w.Write(content)
			}
			return
		}

		fileServer.ServeHTTP(w, r)
	}

	http.HandleFunc("/", gzipHandler(fileHandler))

	fmt.Printf("🌍 Serving on http://%s\n", addr)
	if *host == "0.0.0.0" {
		fmt.Println("   (Accessible on your local network)")
	}
	fmt.Println("   (Auto-reload enabled via /events)")

	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

// Helper: recursive walk to find the latest modification time in the directory
func getLatestModTime(dir string) (time.Time, error) {
	var latest time.Time
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
		return nil
	})
	return latest, err
}
