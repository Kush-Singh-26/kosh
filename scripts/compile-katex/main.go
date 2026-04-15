package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fastschema/qjs"
	"github.com/zeebo/xxh3"
	"runtime"
)

const (
	katexHeaderSize    = 20
	katexMagic         = "KBC1"
	katexMagicSize     = 4
	katexHashStart     = 4
	katexHashEnd       = 12
	katexSizeStart     = 12
	katexSizeEnd       = 20
	katexOutputDirMode = 0755
)

func main() {
	// Read katex.min.js from the native package directory
	_, filename, _, _ := runtime.Caller(0)
	scriptDir := filepath.Dir(filename)
	jsPath := filepath.Join(scriptDir, "../../builder/renderer/native/katex.min.js")
	jsData, err := os.ReadFile(jsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read katex.min.js at %s: %v\n", jsPath, err)
		os.Exit(1)
	}

	// Calculate hash of source JS
	jsHash := xxh3.Hash(jsData)

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
		rt.Close()
		os.Exit(1) //nolint:gocritic
	}

	// Prepare header (20 bytes)
	// 4 bytes: Magic "KBC1"
	// 8 bytes: Source JS Hash
	// 8 bytes: Bytecode size (for integrity check)
	header := make([]byte, katexHeaderSize)
	copy(header[0:katexMagicSize], katexMagic)
	binary.LittleEndian.PutUint64(header[katexHashStart:katexHashEnd], jsHash)
	binary.LittleEndian.PutUint64(header[katexSizeStart:katexSizeEnd], uint64(len(bytecode)))

	// Get the output path from command line or use default
	outputPath := filepath.Join(scriptDir, "../../builder/renderer/native/katex.bytecode")
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}

	// Ensure the output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, katexOutputDirMode); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Write header + bytecode to file
	f, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close output file: %v\n", err)
		}
	}()

	if _, err := f.Write(header); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write header: %v\n", err)
		os.Exit(1)
	}

	if _, err := f.Write(bytecode); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write bytecode: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully compiled KaTeX to bytecode with header: %s (%d bytes total)\n", outputPath, len(header)+len(bytecode))
}
