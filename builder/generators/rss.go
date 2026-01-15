package generators

import (
	"encoding/xml"
	"fmt"
	"os"
	"time"

	"my-ssg/builder/models"
)

func GenerateRSS(baseURL string, posts []models.PostMetadata) {
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
			Title:       "Kush Blogs",
			Link:        baseURL,
			Description: "ML & Deep Learning Blog",
			Items:       items,
		},
	}
	output, _ := xml.MarshalIndent(rss, "", "  ")
	if err := os.WriteFile("public/rss.xml", []byte(xml.Header+string(output)), 0644); err != nil {
		fmt.Printf("⚠️ Failed to write rss.xml: %v\n", err)
	}
}
