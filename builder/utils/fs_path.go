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
	if path == "" || path == "." {
		return "."
	}

	// Fast path for already clean paths with forward slashes
	if !strings.Contains(path, "\\") && !strings.Contains(path, "//") && !strings.Contains(path, "./") {
		// Just check for drive letter on Windows
		if runtime.GOOS == "windows" && len(path) >= 2 && path[1] == ':' && path[0] >= 'a' && path[0] <= 'z' {
			return strings.ToUpper(path[:1]) + path[1:]
		}
		return path
	}

	// Apply NFC normalization for consistent Unicode handling across platforms
	path = norm.NFC.String(path)

	// Clean the path and convert to forward slashes for internal consistency
	path = filepath.ToSlash(filepath.Clean(path))

	// On Windows, capitalize drive letter for consistency (e.g., "c:" -> "C:")
	if runtime.GOOS == "windows" {
		if len(path) >= 2 && path[1] == ':' {
			if path[0] >= 'a' && path[0] <= 'z' {
				path = strings.ToUpper(path[:1]) + path[1:]
			}
		}
	}

	return path
}

func SafeRel(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}

	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("path traversal detected: result escapes base directory")
	}

	if strings.Contains(rel, "\\") {
		return filepath.ToSlash(rel), nil
	}
	return rel, nil
}

func WriteFileVFS(fs afero.Fs, path string, data []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return afero.WriteFile(fs, path, data, 0644)
}

func GetRelativePrefix(htmlPath string) string {
	if len(htmlPath) == 0 {
		return ""
	}

	// Clean path and ensure forward slashes
	path := filepath.ToSlash(filepath.Clean(htmlPath))

	// Ignore leading slash for depth calculation
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	if path == "index.html" || path == "." || path == "" {
		return ""
	}

	depth := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			depth++
		}
	}

	if depth == 0 {
		return ""
	}

	switch depth {
	case 1:
		return "../"
	case 2:
		return "../../"
	case 3:
		return "../../../"
	default:
		return strings.Repeat("../", depth)
	}
}
