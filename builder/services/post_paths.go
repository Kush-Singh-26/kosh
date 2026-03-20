package services

import (
	"path/filepath"
	"strings"
)

// ComputePathVars derives all path variants needed for post processing.
// relPath should be the full relative path from the content directory (including version prefix if any).
// Returns: htmlRelPath, cleanHtmlRelPath, destPath
func ComputePathVars(outputDir, relPath, version string) (htmlRelPath, cleanHtmlRelPath, destPath string) {
	// Normalize to forward slashes for consistent prefix stripping
	relPath = filepath.ToSlash(relPath)
	htmlRelPath = strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

	cleanHtmlRelPath = htmlRelPath
	if version != "" {
		versionPrefix := strings.ToLower(version) + "/"
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, versionPrefix)
	}

	if version != "" {
		destPath = filepath.Join(outputDir, version, cleanHtmlRelPath)
	} else {
		destPath = filepath.Join(outputDir, htmlRelPath)
	}
	return
}

// CardPaths computes paths and URLs for a post's social card.
func CardPaths(baseURL, outputDir, htmlRelPath string) (cardRelPath, cardDestPath, cardImageURL string) {
	cardRelPath = strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	cardDestPath = filepath.ToSlash(filepath.Join(outputDir, "static", "images", "cards", cardRelPath))
	cardImageURL = baseURL + "/static/images/cards/" + cardRelPath
	return
}
