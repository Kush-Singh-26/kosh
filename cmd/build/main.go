// build.go handles conditional building of WASM and regular binaries
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	// Ensure bin directory exists
	os.MkdirAll("bin", 0755)

	// 1. Conditional Site Builder Build
	builderSrc := []string{"builder", "cmd/builder"}
	builderOut := "./bin/builder.exe"

	if needsRebuild(builderSrc, builderOut) {
		fmt.Println("🔨 Building site builder...")
		if err := run("go", "build", "-o", builderOut, "./cmd/builder"); err != nil {
			fmt.Printf("❌ Builder build failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("⏭️  Builder source unchanged. Skipping build.")
	}

	// 2. Conditional WASM Build
	wasmSrc := []string{
		"cmd/search",
		"builder/search",
		"builder/models",
	}
	wasmOut := "static/wasm/search.wasm"

	if needsRebuild(wasmSrc, wasmOut) {
		fmt.Println("🚀 WASM source changes detected. Building Search WASM...")
		cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", wasmOut, "./cmd/search/main.go")
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ WASM build failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ WASM build complete.")
	} else {
		fmt.Println("⏭️  WASM source unchanged. Skipping build.")
	}
}

func needsRebuild(srcs []string, out string) bool {
	outInfo, err := os.Stat(out)
	if err != nil {
		return true // Output file doesn't exist
	}
	outTime := outInfo.ModTime()

	for _, src := range srcs {
		err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && info.ModTime().After(outTime) {
				return fmt.Errorf("rebuild needed")
			}
			return nil
		})
		if err != nil {
			return true
		}
	}
	return false
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
