package navigation

import (
	"path/filepath"
	"strings"
)

type PostPaths struct {
	HTMLRelPath      string
	CleanHTMLRelPath string
	DestPath         string
	Link             string
	CardRelPath      string
	CardDestPath     string
	CardImageURL     string
}

func ComputePathVars(outputDir, relPath string) (htmlRelPath, cleanHtmlRelPath, destPath string) {
	relPath = filepath.ToSlash(relPath)
	htmlRelPath = strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

	cleanHtmlRelPath = htmlRelPath
	destPath = filepath.Join(outputDir, htmlRelPath)
	return
}

func BuildAbsoluteURL(baseURL, relPath string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	relPath = strings.TrimPrefix(relPath, "/")
	return baseURL + "/" + relPath
}

func CardPaths(baseURL, outputDir, htmlRelPath string) (cardRelPath, cardDestPath, cardImageURL string) {
	cardRelPath = strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	cardDestPath = filepath.ToSlash(filepath.Join(outputDir, "static", "images", "cards", cardRelPath))
	cardImageURL = baseURL + "/static/images/cards/" + cardRelPath
	return
}
