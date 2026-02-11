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

	// Version-aware linking: If we're in a versioned file and the link is relative,
	// prepend the version prefix to keep the user in the same version context
	if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "http") {
		if filePath, ok := pc.Get(ContextKeyFilePath).(string); ok && filePath != "" {
			// Check if current file is in a version directory
			version := extractVersionFromPath(filePath)
			if version != "" {
				// Check if link already has version prefix or goes to a different version
				if !strings.HasPrefix(href, version+"/") && !isCrossVersionLink(href) {
					// Check if this is a parent directory traversal that goes outside version
					if strings.HasPrefix(href, "../") {
						// Count ../ to see if we go above version root
						depth := countParentRefs(href)
						fileDepth := getFileDepthInVersion(filePath)
						if depth <= fileDepth {
							// Still within version context, prepend version
							href = version + "/" + href
						}
						// If depth > fileDepth, we're going to root, don't add version
					} else {
						// Simple relative link, prepend version
						href = version + "/" + href
					}
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
	// Check for patterns like "../v1.0/" or "../v2.0/"
	return strings.Contains(href, "/v") && strings.Contains(href, "..")
}

// countParentRefs counts the number of "../" at the start of a path
func countParentRefs(href string) int {
	count := 0
	for strings.HasPrefix(href, "../") {
		count++
		href = href[3:]
	}
	return count
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
