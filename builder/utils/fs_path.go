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
	path = norm.NFC.String(path)

	// Clean the path and convert to forward slashes for internal consistency
	path = filepath.ToSlash(filepath.Clean(path))

	// On Windows, capitalize drive letter for consistency (e.g., "c:" -> "C:")
	if runtime.GOOS == "windows" {
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

func GetRelativePrefix(htmlPath string) string {
	htmlPath = filepath.ToSlash(htmlPath)
	// If path is absolute or has a drive letter, it's not a relative path from output root
	if filepath.IsAbs(htmlPath) || (len(htmlPath) > 1 && htmlPath[1] == ':') || strings.HasPrefix(htmlPath, "C:") || strings.HasPrefix(htmlPath, "c:") {
		return ""
	}

	htmlPath = strings.TrimPrefix(htmlPath, "./")
	htmlPath = strings.TrimPrefix(htmlPath, "/")

	depth := strings.Count(htmlPath, "/")
	if depth == 0 {
		return ""
	}
	return strings.Repeat("../", depth)
}
