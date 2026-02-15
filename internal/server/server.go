package server

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Run(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "localhost", "The host/IP to bind to")
	port := fs.String("port", "2604", "The port to listen on")

	_ = fs.Bool("drafts", false, "Include drafts (handled by builder)")
	_ = fs.String("baseurl", "", "Base URL (handled by builder)")
	_ = fs.Bool("compress", false, "Enable compression (handled by builder)")

	_ = fs.Parse(args)

	addr := fmt.Sprintf("%s:%s", *host, *port)

	_ = mime.AddExtensionType(".wasm", "application/wasm")

	staticDir := "./public"

	startWatcher(staticDir)
	defer stopWatcher()

	go func() {
		<-ctx.Done()
		fmt.Println("\n🛑 Shutting down server...")
		stopWatcher()
	}()

	fileServer := http.FileServer(http.Dir(staticDir))

	http.HandleFunc("/events", handleSSE)

	http.HandleFunc("/", gzipHandler(func(w http.ResponseWriter, r *http.Request) {
		rawPath := r.URL.Path
		normalizedPath := normalizeRequestPath(rawPath)

		fullPath, err := validatePath(staticDir, normalizedPath)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("403 - Forbidden: Invalid path"))
			return
		}

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
	}))

	go broadcastReload()

	httpServer := &http.Server{
		Addr:    addr,
		Handler: nil,
	}

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
