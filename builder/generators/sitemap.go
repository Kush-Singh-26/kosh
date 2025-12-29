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
    for t := range tags {
        urls = append(urls, models.Url{
            Loc: fmt.Sprintf("%s/tags/%s.html", baseURL, t),
        })
    }

    // Marshaling
    output, err := xml.MarshalIndent(models.UrlSet{Urls: urls}, "", "  ")
    if err != nil {
        fmt.Printf("Error marshaling sitemap: %v\n", err)
        return
    }

    finalOutput := []byte(xml.Header + string(output))
    
    os.WriteFile("public/sitemap.xml", finalOutput, 0644)
}