// DebugPath prints directory structure information for troubleshooting.
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func main() {
	baseDir := "public"
	userPath := "/static/js/wasm_exec.js"

	cleanUserPath := filepath.ToSlash(filepath.Clean(userPath))
	trimmedPath := strings.TrimLeft(cleanUserPath, "/")

	absBase, _ := filepath.Abs(baseDir)
	absUserPath, _ := filepath.Abs(filepath.Join(absBase, filepath.FromSlash(trimmedPath)))

	fmt.Printf("Base: %s\n", absBase)
	fmt.Printf("Joined: %s\n", absUserPath)

	if absUserPath != absBase && !strings.HasPrefix(absUserPath, absBase+string(filepath.Separator)) {
		fmt.Println("TRAVERSAL ERROR")
	} else {
		fmt.Println("PATH OK")
	}
}
