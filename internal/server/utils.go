package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validatePath ensures that the user-provided path is within the base directory
// and prevents path traversal attacks.
func validatePath(baseDir, userPath string) (string, error) {
	// Convert to absolute paths to prevent any relative traversal
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	// Clean the user path to remove any ../ or ./
	cleanPath := filepath.Clean(userPath)

	// Join with base directory and convert to absolute
	absUserPath, err := filepath.Abs(filepath.Join(baseDir, cleanPath))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the resolved path is within the base directory
	// Use strings.HasPrefix to handle both Unix and Windows path separators
	if !strings.HasPrefix(absUserPath, absBase) {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	// Additional security check: ensure it's not trying to escape back to parent
	relPath, err := filepath.Rel(absBase, absUserPath)
	if err != nil {
		return "", fmt.Errorf("path validation error: %w", err)
	}

	// Check for any ".." in the relative path which could indicate traversal
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	return absUserPath, nil
}

// normalizeRequestPath normalizes the request path for cross-platform consistency
// and applies any necessary transformations.
func normalizeRequestPath(rawPath string) string {
	// Trim blog prefix if present
	if strings.HasPrefix(rawPath, "/blogs/") {
		rawPath = strings.TrimPrefix(rawPath, "/blogs/")
	}

	// Convert to forward slashes and clean the path
	return filepath.ToSlash(filepath.Clean(rawPath))
}
