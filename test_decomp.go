package main

import (
	"fmt"
	"io"
	"os"

	"github.com/andybalholm/brotli"
)

func main() {
	f, err := os.Open("public/search.bin")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer f.Close()

	br := brotli.NewReader(f)

	n, err := io.Copy(io.Discard, br)
	if err != nil {
		fmt.Println("Error decompressing:", err)
		return
	}

	fmt.Printf("Successfully decompressed %d bytes\n", n)
}
