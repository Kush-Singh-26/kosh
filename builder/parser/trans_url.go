package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ContextKeyFilePath stores the current file path being parsed
var ContextKeyFilePath = parser.NewContextKey()

// URLTransformer intercepts links and images to rewrite URLs (e.g., .md -> .html).
type URLTransformer struct {
	BaseURL string
}

func (t *URLTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch target := n.(type) {
		case *ast.Link:
			t.processDestination(target, target.Destination, pc)
		case *ast.Image:
			t.processDestination(target, target.Destination, pc)
		}
		return ast.WalkContinue, nil
	})
}

func (t *URLTransformer) processDestination(n ast.Node, dest []byte, pc parser.Context) {
	href := string(dest)

	// Handle External Links
	if strings.HasPrefix(href, "http") {
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	} else {
		ext := strings.ToLower(filepath.Ext(href))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			href = href[:len(href)-len(ext)] + ".webp"
			switch node := n.(type) {
			case *ast.Link:
				node.Destination = []byte(href)
			case *ast.Image:
				node.Destination = []byte(href)
			}
		}
	}

	// Convert .md to .html
	if strings.HasSuffix(href, ".md") && !strings.HasPrefix(href, "http") {
		href = strings.Replace(href, ".md", ".html", 1)
		href = strings.ToLower(href)
	}

	// Clean up ./ prefix which is redundant
	href = strings.TrimPrefix(href, "./")

	// Version-aware linking: Handle relative links within versioned documentation
	// Option A: Use relative paths without version prefix for same-version links
	if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "http") {
		if filePath, ok := pc.Get(ContextKeyFilePath).(string); ok && filePath != "" {
			version := extractVersionFromPath(filePath)
			if version != "" {
				// Don't modify cross-version links (../v1.0/, ../v3.0/, etc.)
				if isCrossVersionLink(href) {
					// Keep as-is (e.g., ../v1.0/page.md → ../v1.0/page.html)
				} else if isRootLevelLink(href) {
					// Root-level links go to root (e.g., ../index.md → ../index.html)
					// Keep as-is
				} else {
					// Same-version links: strip ../ prefix if present, keep relative
					// e.g., ./new-in-v2.md → new-in-v2.html
					// e.g., ./advanced/setup.md → advanced/setup.html
					// e.g., ../advanced/config.md → advanced/config.html
					href = strings.TrimPrefix(href, "../")
					// Ensure forward slashes
					href = strings.ReplaceAll(href, "\\", "/")
				}
			}
		}
	}

	// Apply the href changes to the node
	if !strings.HasPrefix(string(dest), "http") {
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = []byte(href)
		case *ast.Image:
			node.Destination = []byte(href)
		}
	}

	if _, isImage := n.(*ast.Image); isImage {
		n.SetAttribute([]byte("loading"), []byte("lazy"))
	}

	if strings.HasPrefix(href, "/") && t.BaseURL != "" {
		newDest := []byte(t.BaseURL + href)
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = newDest
		case *ast.Image:
			node.Destination = newDest
		}
	}
}

// extractVersionFromPath extracts version from file path like "content/v2.0/page.md"
func extractVersionFromPath(path string) string {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if i == 0 {
			continue // Skip "content"
		}
		if strings.HasPrefix(part, "v") && len(part) > 2 {
			return part
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
	if strings.HasPrefix(href, "../") {
		trimmed := strings.TrimPrefix(href, "../")
		parts := strings.Split(trimmed, "/")
		for _, part := range parts {
			if strings.HasPrefix(part, "v") && len(part) > 2 {
				return true
			}
		}
	}
	return false
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
		// Check if filename matches common root-level files
		rootFiles := []string{
			"index", "features", "getting-started",
			"docs", "guide", "help", "readme", "intro",
		}
		for _, rf := range rootFiles {
			if trimmed == rf {
				return true
			}
		}
	}
	return false
}

// getFileDepthInVersion returns how many directories deep a file is within its version
func getFileDepthInVersion(filePath string) int {
	path := filepath.ToSlash(filePath)
	parts := strings.Split(path, "/")
	versionIdx := -1
	for i, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) > 2 {
			versionIdx = i
			break
		}
	}
	if versionIdx == -1 {
		return 0
	}
	// Count directories after version (excluding filename)
	return len(parts) - versionIdx - 2
}
