package main

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// 1. Force register the WASM mime type
	mime.AddExtensionType(".wasm", "application/wasm")

	port := ":8080"
	
	// Define the directory to serve
	staticDir := "./public"
	fs := http.FileServer(http.Dir(staticDir))

	// 2. Wrap the FileServer in a custom handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent directory traversal attacks
		path := filepath.Clean(r.URL.Path)

		// Construct the full path to the file on disk
		fullPath := filepath.Join(staticDir, path)

		// Check if the file (or directory) exists
		_, err := os.Stat(fullPath)
		
		if os.IsNotExist(err) {
			// FILE DOES NOT EXIST: Serve 404.html
			w.WriteHeader(http.StatusNotFound)
			http.ServeFile(w, r, filepath.Join(staticDir, "404.html"))
			return
		}

		// FILE EXISTS: Let the standard FileServer handle it
		fs.ServeHTTP(w, r)
	})

	fmt.Printf("🌍 Serving locally on http://localhost%s\n", port)
	fmt.Println("Press Ctrl+C to stop")

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatal(err)
	}
}