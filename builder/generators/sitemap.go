package generators

import (
	"encoding/xml"
	"fmt"
	"os"
	"time"

	"my-ssg/builder/models"
)

func GenerateSitemap(baseURL string, posts []models.PostMetadata, tags map[string][]models.PostMetadata) {
	var urls []models.Url
	urls = append(urls, models.Url{Loc: baseURL + "/", LastMod: time.Now().Format("2006-01-02")})
	for _, p := range posts {
		urls = append(urls, models.Url{Loc: p.Link, LastMod: p.DateObj.Format("2006-01-02")})
	}
	for t := range tags {
		urls = append(urls, models.Url{Loc: fmt.Sprintf("%s/tags/%s.html", baseURL, t)})
	}
	output, _ := xml.MarshalIndent(models.UrlSet{Urls: urls}, "  ", "    ")
	os.WriteFile("public/sitemap.xml", []byte(xml.Header+string(output)), 0644)
}