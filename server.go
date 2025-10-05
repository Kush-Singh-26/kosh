// blog_temp/server.go
package main

import (
	"log"
	"net/http"
)

func main() {
	// Define the directory to serve
	dir := "public"
	
	// Define the port
	port := "8000"

	// Create a file server handler
	fs := http.FileServer(http.Dir(dir))

	// Handle all requests by serving a file out of the public directory
	http.Handle("/", fs)

	// Print a message to the console
	log.Printf("Serving %s on http://localhost:%s\n", dir, port)

	// Start the server and log any errors
	log.Fatal(http.ListenAndServe(":"+port, nil))
}