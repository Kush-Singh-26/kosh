package generators

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func GenerateRSS(destFs afero.Fs, baseURL string, posts []models.PostMetadata, title, description string, outputPath string) {
	fmt.Println("📡 Generating RSS feed...")

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
	output, _ := xml.MarshalIndent(rss, "", "  ")
	if err := utils.WriteFileVFS(destFs, outputPath, []byte(xml.Header+string(output))); err != nil {
		fmt.Printf("⚠️ Failed to write rss.xml: %v\n", err)
	}
}
