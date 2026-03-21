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

func ComputePathVars(outputDir, relPath, version string) (htmlRelPath, cleanHtmlRelPath, destPath string) {
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

func CardPaths(baseURL, outputDir, htmlRelPath string) (cardRelPath, cardDestPath, cardImageURL string) {
	cardRelPath = strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	cardDestPath = filepath.ToSlash(filepath.Join(outputDir, "static", "images", "cards", cardRelPath))
	cardImageURL = baseURL + "/static/images/cards/" + cardRelPath
	return
}
