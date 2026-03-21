package generators

import (
	"fmt"
	"log/slog"
	"strings"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

func GenerateRobotsTxt(sink fspkg.ArtifactSink, baseURL string, outputPath string) (string, error) {
	slog.Info("Generating robots.txt")

	baseURL = strings.TrimRight(baseURL, "/")
	content := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap/sitemap.xml\n", baseURL)

	if err := sink.WriteFile(outputPath, []byte(content)); err != nil {
		return "", err
	}

	return outputPath, nil
}
