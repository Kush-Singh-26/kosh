package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
)

var (
	BaseURL        string
	CompressImages bool
	ForceRebuild   bool
)

/* -------------------- Data Structures -------------------- */

type PostMetadata struct {
	Title, Link, Description string
	Tags                     []string
	ReadingTime              int
	Pinned                   bool
	DateObj                  time.Time
	HasMath                  bool
}

type TagData struct {
	Name, Link string
	Count      int
}

type PageData struct {
	Title, Description, BaseURL string
	Content                     template.HTML
	Meta                        map[string]interface{}
	IsIndex, IsTagsIndex        bool
	Posts                       []PostMetadata
	PinnedPosts                 []PostMetadata
	AllTags                     []TagData
	BuildVersion                int64
	HasMath                     bool
	LayoutCSS                   template.CSS
	ThemeCSS                    template.CSS

	// SEO
	Permalink string
	Image     string
}

/* -------------------- Sitemap -------------------- */

type UrlSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	Urls    []Url    `xml:"url"`
}

type Url struct {
	Loc, LastMod string
}

/* -------------------- RSS -------------------- */

type Rss struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Items       []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Guid        string `xml:"guid"`
}

/* -------------------- AST Transformer -------------------- */

type URLTransformer struct{}

func (t *URLTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch target := n.(type) {
		case *ast.Link:
			processDestination(target, target.Destination)
		case *ast.Image:
			processDestination(target, target.Destination)
		}
		return ast.WalkContinue, nil
	})
}

func processDestination(n ast.Node, dest []byte) {
	href := string(dest)

	if strings.HasPrefix(href, "http") {
		if link, ok := n.(*ast.Link); ok {
			link.SetAttribute([]byte("target"), []byte("_blank"))
			link.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	}

	if img, ok := n.(*ast.Image); ok {
		img.SetAttribute([]byte("loading"), []byte("lazy"))
	}

	if strings.HasPrefix(href, "/") && BaseURL != "" {
		newDest := []byte(BaseURL + href)
		switch t := n.(type) {
		case *ast.Link:
			t.Destination = newDest
		case *ast.Image:
			t.Destination = newDest
		}
	}
}

/* -------------------- Main -------------------- */

func main() {
	baseUrlFlag := flag.String("baseurl", "", "Base URL")
	compressFlag := flag.Bool("compress", false, "Enable image compression")
	flag.Parse()

	BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	CompressImages = *compressFlag

	buildVersion := time.Now().Unix()
	fmt.Printf("🔨 Building site... (Version: %d)\n", buildVersion)

	layoutInfo, err := os.Stat("templates/layout.html")
	if err != nil {
		log.Fatal("templates/layout.html not found")
	}

	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		if layoutInfo.ModTime().After(indexInfo.ModTime()) {
			ForceRebuild = true
		}
	} else {
		ForceRebuild = true
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(highlighting.WithStyle("nord")),
			passthrough.New(passthrough.Config{
				InlineDelimiters: []passthrough.Delimiters{
					{Open: "$", Close: "$"},
					{Open: "\\(", Close: "\\)"},
				},
				BlockDelimiters: []passthrough.Delimiters{
					{Open: "$$", Close: "$$"},
					{Open: "\\[", Close: "\\]"},
				},
			}),
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(&URLTransformer{}, 100)),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	os.MkdirAll("public/tags", 0755)

	if _, err := os.Stat("static"); err == nil {
		copyDir("static", "public/static")
	}

	layoutBytes, _ := os.ReadFile("static/css/layout.css")
	themeBytes, _ := os.ReadFile("static/css/theme.css")

	layoutCSS := template.CSS(layoutBytes)
	themeCSS := template.CSS(themeBytes)

	funcMap := template.FuncMap{"lower": strings.ToLower}
	tmpl := template.Must(template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html"))

	var allPosts, pinnedPosts []PostMetadata
	tagMap := make(map[string][]PostMetadata)

	filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}

		rel, _ := filepath.Rel("content", path)
		htmlRel := strings.ToLower(strings.Replace(rel, ".md", ".html", 1))
		dest := filepath.Join("public", htmlRel)
		link := BaseURL + "/" + htmlRel

		source, _ := os.ReadFile(path)
		var buf bytes.Buffer
		ctx := parser.NewContext()
		md.Convert(source, &buf, parser.WithContext(ctx))

		metaData := meta.Get(ctx)

		wordCount := len(strings.Fields(string(source)))
		readTime := int(math.Ceil(float64(wordCount) / 120))

		dateObj, _ := time.Parse("2006-01-02", getString(metaData, "date"))
		hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")

		post := PostMetadata{
			Title:       getString(metaData, "title"),
			Description: getString(metaData, "description"),
			Tags:        getSlice(metaData, "tags"),
			Link:        link,
			ReadingTime: readTime,
			Pinned:      metaData["pinned"] == true,
			DateObj:     dateObj,
			HasMath:     hasMath,
		}

		image := BaseURL + "/static/images/favicon.ico"
		if img, ok := metaData["image"].(string); ok {
			image = BaseURL + img
		}

		renderPage(tmpl, dest, PageData{
			Title:       post.Title,
			Description: post.Description,
			Content:     template.HTML(buf.String()),
			BaseURL:     BaseURL,
			BuildVersion: buildVersion,
			Permalink:   link,
			Image:       image,
			HasMath:     post.HasMath,
			LayoutCSS:   layoutCSS,
			ThemeCSS:    themeCSS,
		})

		if post.Pinned {
			pinnedPosts = append(pinnedPosts, post)
		} else {
			allPosts = append(allPosts, post)
		}

		for _, t := range post.Tags {
			tagMap[strings.ToLower(t)] = append(tagMap[strings.ToLower(t)], post)
		}

		return nil
	})

	sortPosts(allPosts)
	sortPosts(pinnedPosts)

	allContent := append(pinnedPosts, allPosts...)
	generateSitemap(allContent, tagMap)
	generateRSS(allContent)

	fmt.Println("✅ Build Complete.")
}

/* -------------------- Helpers -------------------- */

func sortPosts(posts []PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].DateObj.After(posts[j].DateObj)
	})
}

func renderPage(t *template.Template, path string, data PageData) {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	if err := t.Execute(f, data); err != nil {
		log.Fatal(err)
	}
}

func generateSitemap(posts []PostMetadata, tags map[string][]PostMetadata) {
	var urls []Url
	urls = append(urls, Url{Loc: BaseURL + "/", LastMod: time.Now().Format("2006-01-02")})
	for _, p := range posts {
		urls = append(urls, Url{Loc: p.Link, LastMod: p.DateObj.Format("2006-01-02")})
	}
	output, _ := xml.MarshalIndent(UrlSet{Urls: urls}, "", "  ")
	os.WriteFile("public/sitemap.xml", []byte(xml.Header+string(output)), 0644)
}

func generateRSS(posts []PostMetadata) {
	var items []Item
	for _, p := range posts {
		items = append(items, Item{
			Title:       p.Title,
			Link:        p.Link,
			Description: p.Description,
			PubDate:     p.DateObj.Format(time.RFC1123),
			Guid:        p.Link,
		})
	}

	rss := Rss{
		Version: "2.0",
		Channel: Channel{
			Title:       "Kush Blogs",
			Link:        BaseURL,
			Description: "I write about machine learning, deep learning and NLP.",
			Items:       items,
		},
	}

	out, _ := xml.MarshalIndent(rss, "", "  ")
	os.WriteFile("public/rss.xml", []byte(xml.Header+string(out)), 0644)
}

func getString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getSlice(m map[string]interface{}, k string) []string {
	var res []string
	if v, ok := m[k].([]interface{}); ok {
		for _, i := range v {
			res = append(res, fmt.Sprintf("%v", i))
		}
	}
	return res
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dest := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}

		ext := strings.ToLower(filepath.Ext(path))
		if (ext == ".jpg" || ext == ".jpeg" || ext == ".png") && CompressImages {
			return processImage(path, dest, ext)
		}
		return copyFileStandard(path, dest)
	})
}

func processImage(srcPath, dstPath, ext string) error {
	img, err := imaging.Open(srcPath)
	if err != nil {
		return copyFileStandard(srcPath, dstPath)
	}

	if img.Bounds().Dx() > 1200 {
		img = imaging.Resize(img, 1200, 0, imaging.Lanczos)
	}

	if ext == ".png" {
		return imaging.Save(img, dstPath, imaging.PNGCompressionLevel(png.BestCompression))
	}
	return imaging.Save(img, dstPath, imaging.JPEGQuality(75))
}

func copyFileStandard(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = io.Copy(d, s)
	return err
}
