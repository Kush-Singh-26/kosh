package navigation

import (
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/fs"
)

// PostPaths holds computed paths for a post and its social card.
type PostPaths struct {
	HTMLRelPath      string
	CleanHTMLRelPath string
	DestPath         string
	Link             string
	CardRelPath      string
	CardDestPath     string
	CardImageURL     string
}

// ComputePathVars returns HTML-relative and destination paths for a post.
func ComputePathVars(outputDir, relPath string) (htmlRelPath, cleanHtmlRelPath, destPath string) {
	relPath = filepath.ToSlash(relPath)
	htmlRelPath = fs.MarkdownToHTMLPath(relPath)

	cleanHtmlRelPath = htmlRelPath
	destPath = filepath.Join(outputDir, htmlRelPath)
	return
}

// BuildAbsoluteURL joins a base URL with a relative path.
func BuildAbsoluteURL(baseURL, relPath string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	relPath = strings.TrimPrefix(relPath, "/")
	return baseURL + "/" + relPath
}

// CardPaths computes social card paths and URLs for a post.
func CardPaths(baseURL, outputDir, htmlRelPath string) (cardRelPath, cardDestPath, cardImageURL string) {
	cardRelPath = strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	cardDestPath = filepath.ToSlash(filepath.Join(outputDir, "static", "images", "cards", cardRelPath))
	cardImageURL = baseURL + "/static/images/cards/" + cardRelPath
	return
}
