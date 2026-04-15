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
	sink := opts.Sink
	baseURL := opts.BaseURL
	posts := opts.Posts
	title := opts.Title
	description := opts.Description
	outputPath := opts.OutputPath

	slog.Info("Generating RSS feed")

	var items []models.Item
	var lastBuildDate string
	if len(posts) > 0 {
		lastBuildDate = posts[0].DateObj.Format(time.RFC1123)
	}

	for _, p := range posts {
		allTerms := []string{}
		for _, terms := range p.Taxonomies {
			allTerms = append(allTerms, terms...)
		}

		item := models.Item{
			Title:       p.Title,
			Link:        p.Link,
			Description: p.Description,
			PubDate:     p.DateObj.Format(time.RFC1123),
			GUID:        p.Link,
			Author:      opts.Author,
			Categories:  allTerms,
		}

		if p.ContentHTML != "" {
			item.ContentEncoded = p.ContentHTML
		}
		items = append(items, item)
	}

	rss := models.Rss{
		Version:      "2.0",
		XMLNSContent: "http://purl.org/rss/1.0/modules/content/",
		Channel: models.Channel{
			Title:         title,
			Link:          baseURL,
			Description:   description,
			LastBuildDate: lastBuildDate,
			Items:         items,
		},
	}

	if opts.LogoURL != "" {
		rss.Channel.Image = &models.RSSImage{
			URL:   opts.LogoURL,
			Title: title,
			Link:  baseURL,
		}
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
