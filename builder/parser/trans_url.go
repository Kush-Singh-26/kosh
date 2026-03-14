package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/parser"
)

// ContextKeyFilePath stores the current file path being parsed.
// Isolated via parser.NewContextKey() which guarantees global uniqueness.
var ContextKeyFilePath = parser.NewContextKey()

// extractVersionFromPath extracts version from file path like "content/v2.0/page.md"
func extractVersionFromPath(path string) string {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if i == 0 {
			continue // Skip "content"
		}
		if strings.HasPrefix(part, "v") && len(part) > 2 {
			if part[1] >= '0' && part[1] <= '9' {
				return part
			}
		}
	}
	return ""
}

// isCrossVersionLink checks if link explicitly references another version
func isCrossVersionLink(href string) bool {
	// Check for patterns like "../v1.0/" or "../v3.0/"
	if strings.Contains(href, "/v") && strings.Contains(href, "..") {
		return true
	}
	// Check if path after ../ starts with a version prefix (e.g., "../v1.0/page.md")
	if after, ok := strings.CutPrefix(href, "../"); ok {
		trimmed := after
		parts := strings.SplitSeq(trimmed, "/")
		for part := range parts {
			if strings.HasPrefix(part, "v") && len(part) > 2 {
				if part[1] >= '0' && part[1] <= '9' {
					return true
				}
			}
		}
	}
	return false
}

// rootFileSet is a pre-computed set for O(1) lookup of root-level file names
var rootFileSet = map[string]struct{}{
	"index":           {},
	"features":        {},
	"getting-started": {},
	"docs":            {},
	"guide":           {},
	"help":            {},
	"readme":          {},
	"intro":           {},
}

// isRootLevelLink checks if a link points to a root-level file
// Root-level files like index.md, features.md, getting-started.md should link to root
func isRootLevelLink(href string) bool {
	// Remove leading ../ or ./
	trimmed := strings.TrimPrefix(href, "../")
	trimmed = strings.TrimPrefix(trimmed, "./")

	// Remove .md or .html extension for comparison
	trimmed = strings.TrimSuffix(trimmed, ".md")
	trimmed = strings.TrimSuffix(trimmed, ".html")

	// Check if it points to a root-level file (no subdirectory)
	if !strings.Contains(trimmed, "/") {
		_, ok := rootFileSet[trimmed]
		return ok
	}
	return false
}
