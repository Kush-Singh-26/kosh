package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/text/unicode/norm"
)

func NormalizePath(path string) string {
	// Apply NFC normalization for consistent Unicode handling across platforms
	// This ensures accented characters are handled consistently (e.g., é vs é)
	path = norm.NFC.String(path)

	// Convert backslashes to forward slashes for consistency
	path = strings.ReplaceAll(path, "\\", "/")

	// Only lowercase on Windows (case-insensitive filesystem)
	// Keep original case on Linux/macOS (case-sensitive filesystems)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
		// Capitalize drive letter for Windows (e.g., "c:" -> "C:")
		if len(path) >= 2 && path[1] == ':' {
			path = strings.ToUpper(path[:1]) + path[1:]
		}
	}

	return path
}

func SafeRel(base, target string) (string, error) {
	base = filepath.FromSlash(NormalizePath(base))
	target = filepath.FromSlash(NormalizePath(target))
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("path traversal detected: result escapes base directory")
	}
	return filepath.ToSlash(rel), nil
}

func WriteFileVFS(fs afero.Fs, path string, data []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	if err := afero.WriteFile(fs, path, data, 0644); err != nil {
		return fmt.Errorf("failed to write VFS file %s: %w", path, err)
	}
	return nil
}
