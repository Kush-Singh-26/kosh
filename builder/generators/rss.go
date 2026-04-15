package generators

import (
	"encoding/xml"
	"io"
	"log/slog"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// RSSOptions configures RSS feed generation.
type RSSOptions struct {
	Sink        fspkg.ArtifactSink
	BaseURL     string
	Posts       []models.PostMetadata
	Title       string
	Description string
	Author      string
	LogoURL     string
	OutputPath  string
}

// GenerateRSS builds and writes the RSS feed.
func GenerateRSS(opts RSSOptions) (string, error) {
	slog.Info("Generating RSS feed")

	items, lastBuildDate := buildRSSItems(opts)
	rss := createRSSObject(opts, items, lastBuildDate)

	err := opts.Sink.WriteStream(opts.OutputPath, func(w io.Writer) error {
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
	return opts.OutputPath, nil
}

func buildRSSItems(opts RSSOptions) ([]models.Item, string) {
	items := make([]models.Item, 0, len(opts.Posts))
	var lastBuildDate string
	if len(opts.Posts) > 0 {
		lastBuildDate = opts.Posts[0].DateObj.Format(time.RFC1123)
	}

	for _, p := range opts.Posts {
		var allTerms []string
		for _, terms := range p.Taxonomies {
			allTerms = append(allTerms, terms...)
		}

		item := models.Item{
			Title:          p.Title,
			Link:           p.Link,
			Description:    p.Description,
			PubDate:        p.DateObj.Format(time.RFC1123),
			GUID:           p.Link,
			Author:         opts.Author,
			Categories:     allTerms,
			ContentEncoded: p.ContentHTML,
		}
		items = append(items, item)
	}
	return items, lastBuildDate
}

func createRSSObject(opts RSSOptions, items []models.Item, lastBuildDate string) models.Rss {
	rss := models.Rss{
		Version:      "2.0",
		XMLNSContent: "http://purl.org/rss/1.0/modules/content/",
		Channel: models.Channel{
			Title:         opts.Title,
			Link:          opts.BaseURL,
			Description:   opts.Description,
			LastBuildDate: lastBuildDate,
			Items:         items,
		},
	}

	if opts.LogoURL != "" {
		rss.Channel.Image = &models.RSSImage{
			URL:   opts.LogoURL,
			Title: opts.Title,
			Link:  opts.BaseURL,
		}
	}
	return rss
}
