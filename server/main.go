package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
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