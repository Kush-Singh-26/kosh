// Package main provides a utility to compile KaTeX to QJS bytecode with a custom header.
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
	jsData, jsHash, err := readSourceJS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	bytecode, err := compileJS(jsData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	outputPath := getOutputPath()
	if err := writeBytecode(outputPath, jsHash, bytecode); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully compiled KaTeX to bytecode with header: %s (%d bytes total)\n", outputPath, katexHeaderSize+len(bytecode))
}

func readSourceJS() ([]byte, uint64, error) {
	_, filename, _, _ := runtime.Caller(0)
	scriptDir := filepath.Dir(filename)
	jsPath := filepath.Join(scriptDir, "../../builder/renderer/native/katex.min.js")
	jsData, err := os.ReadFile(jsPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read katex.min.js at %s: %w", jsPath, err)
	}
	return jsData, xxh3.Hash(jsData), nil
}

func compileJS(jsData []byte) ([]byte, error) {
	rt, err := qjs.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create QJS runtime: %w", err)
	}
	defer rt.Close()

	bytecode, err := rt.Compile("katex.min.js", qjs.Code(string(jsData)))
	if err != nil {
		return nil, fmt.Errorf("failed to compile KaTeX: %w", err)
	}
	return bytecode, nil
}

func getOutputPath() string {
	_, filename, _, _ := runtime.Caller(0)
	scriptDir := filepath.Dir(filename)
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return filepath.Join(scriptDir, "../../builder/renderer/native/katex.bytecode")
}

func writeBytecode(outputPath string, jsHash uint64, bytecode []byte) error {
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, katexOutputDirMode); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	header := make([]byte, katexHeaderSize)
	copy(header[0:katexMagicSize], katexMagic)
	binary.LittleEndian.PutUint64(header[katexHashStart:katexHashEnd], jsHash)
	binary.LittleEndian.PutUint64(header[katexSizeStart:katexSizeEnd], uint64(len(bytecode)))

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := f.Write(bytecode); err != nil {
		return fmt.Errorf("failed to write bytecode: %w", err)
	}
	return nil
}
