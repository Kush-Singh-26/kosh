package generators

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type SitemapOptions struct {
	Sink       fspkg.ArtifactSink
	BaseURL    string
	Posts      []models.PostMetadata
	Tags       map[string][]models.PostMetadata
	OutputPath string
}

func GenerateSitemap(opts SitemapOptions) (string, error) {
	sink := opts.Sink
	baseURL := opts.BaseURL
	posts := opts.Posts
	tags := opts.Tags
	outputPath := opts.OutputPath

	slog.Info("Generating sitemap")

	baseURL = strings.TrimRight(baseURL, "/")

	var urls []models.URL

	// 1. Add Home Page
	urls = append(urls, models.URL{
		Loc:     baseURL + "/",
		LastMod: time.Now().Format("2006-01-02"),
	})

	// 2. Add Blog Posts
	for _, p := range posts {
		urls = append(urls, models.URL{
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

		urls = append(urls, models.URL{
			Loc:     fmt.Sprintf("%s/tags/%s.html", baseURL, url.PathEscape(t)),
			LastMod: latest.Format("2006-01-02"),
		})
	}

	// Streaming encode: write XML header then encode each URL entry directly to the sink writer
	err := sink.WriteStream(outputPath, func(w io.Writer) error {
		if _, err := io.WriteString(w, xml.Header); err != nil {
			return err
		}
		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		return enc.Encode(models.URLSet{URLs: urls})
	})
	if err != nil {
		return "", err
	}
	return outputPath, nil
}
