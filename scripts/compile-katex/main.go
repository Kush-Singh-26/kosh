package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fastschema/qjs"
)

func main() {
	// Read katex.min.js from the native package directory
	jsPath := "C:/Users/KIIT0001/blogs/builder/renderer/native/katex.min.js"
	jsData, err := os.ReadFile(jsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read katex.min.js: %v\n", err)
		os.Exit(1)
	}

	// Create a temporary QJS runtime for compilation
	rt, err := qjs.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create QJS runtime: %v\n", err)
		os.Exit(1)
	}
	defer rt.Close()

	// Compile KaTeX to bytecode
	bytecode, err := rt.Compile("katex.min.js", qjs.Code(string(jsData)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to compile KaTeX: %v\n", err)
		os.Exit(1)
	}

	// Get the output path from command line or use default
	outputPath := "C:/Users/KIIT0001/blogs/builder/renderer/native/katex.bytecode"
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}

	// Ensure the output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Write bytecode to file
	f, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Write(bytecode); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write bytecode: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully compiled KaTeX to bytecode: %s (%d bytes)\n", outputPath, len(bytecode))
}
