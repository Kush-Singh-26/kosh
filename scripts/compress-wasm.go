package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/andybalholm/brotli"
)

func main() {
	wasmPath := "search.wasm"
	brPath := "builder/assets/wasm/search.wasm.br"

	data, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Printf("Failed to read %s: %v\n", wasmPath, err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	bw := brotli.NewWriterLevel(&buf, brotli.BestCompression)
	_, err = bw.Write(data)
	if err != nil {
		fmt.Printf("Failed to compress: %v\n", err)
		os.Exit(1)
	}
	if err := bw.Close(); err != nil {
		fmt.Printf("Failed to close brotli writer: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(brPath, buf.Bytes(), 0644); err != nil {
		fmt.Printf("Failed to write %s: %v\n", brPath, err)
		os.Exit(1)
	}

	fmt.Printf("Compressed %s to %s (%d -> %d bytes)\n", wasmPath, brPath, len(data), buf.Len())
}
