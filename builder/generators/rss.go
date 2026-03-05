package generators

import (
	"encoding/xml"
	"log/slog"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func GenerateRSS(destFs afero.Fs, baseURL string, posts []models.PostMetadata, title, description string, outputPath string) (string, error) {
	slog.Info("Generating RSS feed")

	var items []models.Item
	for _, p := range posts {
		items = append(items, models.Item{
			Title:       p.Title,
			Link:        p.Link,
			Description: p.Description,
			PubDate:     p.DateObj.Format(time.RFC1123),
			Guid:        p.Link,
		})
	}
	rss := models.Rss{
		Version: "2.0",
		Channel: models.Channel{
			Title:       title,
			Link:        baseURL,
			Description: description,
			Items:       items,
		},
	}
	file, err := destFs.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := file.Write([]byte(xml.Header)); err != nil {
		return "", err
	}

	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	if err := enc.Encode(rss); err != nil {
		return "", err
	}
	return outputPath, nil
}
