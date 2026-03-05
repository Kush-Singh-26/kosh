package generators

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func GenerateSitemap(destFs afero.Fs, baseURL string, posts []models.PostMetadata, tags map[string][]models.PostMetadata, outputPath string) (string, error) {
	slog.Info("Generating sitemap")

	var urls []models.Url

	// 1. Add Home Page
	urls = append(urls, models.Url{
		Loc:     baseURL + "/",
		LastMod: time.Now().Format("2006-01-02"),
	})

	// 2. Add Blog Posts
	for _, p := range posts {
		urls = append(urls, models.Url{
			Loc:     p.Link,
			LastMod: p.DateObj.Format("2006-01-02"),
		})
	}

	// 3. Add Tag Pages
	for t, tagPosts := range tags {
		// Find the latest date among posts with this tag
		var latest time.Time
		for _, p := range tagPosts {
			if p.DateObj.After(latest) {
				latest = p.DateObj
			}
		}

		urls = append(urls, models.Url{
			Loc:     fmt.Sprintf("%s/tags/%s.html", baseURL, url.PathEscape(t)),
			LastMod: latest.Format("2006-01-02"),
		})
	}

	// Marshaling
	output, err := xml.MarshalIndent(models.UrlSet{Urls: urls}, "", "  ")
	if err != nil {
		return "", err
	}

	finalOutput := []byte(xml.Header + string(output))
	if err := utils.WriteFileVFS(destFs, outputPath, finalOutput); err != nil {
		return "", err
	}
	return outputPath, nil
}
