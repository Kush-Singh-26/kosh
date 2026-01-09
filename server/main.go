package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// 1. Define flags for Host and Port
	// Default is localhost (private), user can set 0.0.0.0 (public) via CLI
	host := flag.String("host", "localhost", "The host/IP to bind to (use 0.0.0.0 to allow external access)")
	port := flag.String("port", "8080", "The port to listen on")

	// Parse the flags provided in the terminal
	flag.Parse()

	// Create the address string (e.g., "0.0.0.0:8080")
	addr := fmt.Sprintf("%s:%s", *host, *port)

	// 2. Force register the WASM mime type
	mime.AddExtensionType(".wasm", "application/wasm")

	// Define the directory to serve
	staticDir := "./public"
	fs := http.FileServer(http.Dir(staticDir))

	// 3. Wrap the FileServer in a custom handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent directory traversal attacks
		path := filepath.Clean(r.URL.Path)

		// Construct the full path to the file on disk
		fullPath := filepath.Join(staticDir, path)

		// Check if the file (or directory) exists
		_, err := os.Stat(fullPath)

		if os.IsNotExist(err) {
			// FILE DOES NOT EXIST: Serve 404.html manually
			// We set the header first
			w.WriteHeader(http.StatusNotFound)

			// Then read and write the file content manually
			// This avoids http.ServeFile trying to write a 200 OK header afterwards
			notFoundPath := filepath.Join(staticDir, "404.html")
			content, readErr := os.ReadFile(notFoundPath)

			if readErr != nil {
				// Fallback if 404.html is missing
				w.Write([]byte("404 - Page Not Found"))
			} else {
				w.Write(content)
			}
			return
		}

		// FILE EXISTS: Let the standard FileServer handle it
		fs.ServeHTTP(w, r)
	})

	fmt.Printf("🌍 Serving on http://%s\n", addr)
	if *host == "0.0.0.0" {
		fmt.Println("   (Accessible on your local network via your Machine's IP)")
	}
	fmt.Println("Press Ctrl+C to stop")

	// 4. Listen using the dynamic address
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}
