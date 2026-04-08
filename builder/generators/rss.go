package generators

import (
	"encoding/xml"
	"io"
	"log/slog"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type RSSOptions struct {
	Sink        fspkg.ArtifactSink
	BaseURL     string
	Posts       []models.PostMetadata
	Title       string
	Description string
	OutputPath  string
}

func GenerateRSS(opts RSSOptions) (string, error) {
	sink := opts.Sink
	baseURL := opts.BaseURL
	posts := opts.Posts
	title := opts.Title
	description := opts.Description
	outputPath := opts.OutputPath

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
	err := sink.WriteStream(outputPath, func(w io.Writer) error {
		if _, err := w.Write([]byte(xml.Header)); err != nil {
			return err
		}

		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		return enc.Encode(rss)
	})
	if err != nil {
		return "", err
	}
	return outputPath, nil
}
