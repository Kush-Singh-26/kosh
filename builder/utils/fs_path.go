package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/afero"
)

func NormalizePath(path string) string {
	if !strings.Contains(path, "\\") && !strings.HasPrefix(path, "content/") {
		return strings.ToLower(path)
	}

	var b strings.Builder
	b.Grow(len(path))

	skipContent := strings.HasPrefix(path, "content/") || strings.HasPrefix(path, "content\\")
	start := 0
	if skipContent {
		start = 8
	}

	for i := start; i < len(path); i++ {
		c := path[i]
		if c == '\\' {
			b.WriteByte('/')
		} else if c >= 'A' && c <= 'Z' {
			b.WriteByte(c + 32)
		} else {
			b.WriteByte(c)
		}
	}

	result := b.String()

	if runtime.GOOS == "windows" && len(result) >= 2 && result[1] == ':' {
		return strings.ToUpper(result[:1]) + result[1:]
	}

	return result
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
