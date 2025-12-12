package main

import (
	"fmt"
	"log"
	"mime"
	"net/http"
)

func main() {
	// 1. Force register the WASM mime type
	// (Fixes "Incorrect response MIME type" errors in browser)
	mime.AddExtensionType(".wasm", "application/wasm")

	port := ":8080"
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	fmt.Printf("🌍 Serving locally on http://localhost%s\n", port)
	fmt.Println("Press Ctrl+C to stop")
	
	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatal(err)
	}
}