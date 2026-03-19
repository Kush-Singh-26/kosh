package orchestration

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
)

// LogBuildError logs a build error with consistent formatting
func LogBuildError(logger *slog.Logger, err error, context string) {
	logger.Error(context, "error", err)
}

// LogBuildErrorWithPath logs a build error with path context
func LogBuildErrorWithPath(logger *slog.Logger, err error, context, path string) {
	logger.Error(context, "path", path, "error", err)
}

// CheckContext returns false if the context is cancelled, true otherwise
// Use this to avoid repeating select statements for context checking
func CheckContext(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

// IsMarkdownFile returns true if the file has a markdown extension
func IsMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md"
}

// IsAssetFile returns true if the file has a CSS or JS extension
func IsAssetFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".css" || ext == ".js"
}

// IsHTMLFile returns true if the file has an HTML extension
func IsHTMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".html" || ext == ".htm"
}

// IsImageFile returns true if the file has an image extension
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".svg"
}
