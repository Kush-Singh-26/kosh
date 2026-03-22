package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
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

func (t *unifiedTransformer) processImageDestination(img *ast.Image, dest []byte) {
	src := string(dest)
	if src == "" || strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") {
		return
	}
	img.Destination = []byte(strings.ToLower(src))
}

func (t *unifiedTransformer) processDestination(n ast.Node, dest []byte, pc parser.Context) {
	href := string(dest)
	idx := strings.IndexAny(href, "?#")
	query := ""
	if idx != -1 {
		query = href[idx:]
		href = href[:idx]
	}

	if strings.HasPrefix(href, "http") {
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	} else if t.Compress {
		ext := strings.ToLower(filepath.Ext(href))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			href = href[:len(href)-len(ext)] + ".webp"
		}
	}

	if strings.HasSuffix(href, ".md") && !strings.HasPrefix(href, "http") {
		href = strings.Replace(href, ".md", ".html", 1)
		href = strings.ToLower(href)
	}
	href = strings.TrimPrefix(href, "./")

	if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "http") {
		if filePath, ok := pc.Get(ContextKeyFilePath).(string); ok && filePath != "" {
			version := extractVersionFromPath(filePath)
			if version != "" {
				if !isCrossVersionLink(href) && !isRootLevelLink(href) {
					href = strings.TrimPrefix(href, "../")
					href = strings.ReplaceAll(href, "\\", "/")
				}
			}
		}
	}

	fullHref := href + query
	if !strings.HasPrefix(string(dest), "http") {
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = []byte(fullHref)
		case *ast.Image:
			node.Destination = []byte(fullHref)
		}
	}

	if strings.HasPrefix(href, "/") && t.BaseURL != "" {
		newDest := []byte(t.BaseURL + fullHref)
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = newDest
		case *ast.Image:
			node.Destination = newDest
		}
	}
}

func hasTextChild(link *ast.Link, source []byte) bool {
	for child := link.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*ast.Text); ok {
			return true
		}
	}
	return false
}

func getAttrValue(n ast.Node, key string) string {
	attr, _ := n.AttributeString(key)
	if attr == nil {
		return ""
	}
	switch v := attr.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	}
	return ""
}
