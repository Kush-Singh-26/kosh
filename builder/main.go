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
	ForceRebuild   bool // True if layout.html changed
)

// --- Data Structures ---
type PostMetadata struct {
	Title, Link, Description string
	Tags                     []string
	ReadingTime              int
	Pinned                   bool
	DateObj                  time.Time
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
	// SEO Fields
	Permalink                   string
	Image                       string
}

// --- Sitemap Structs ---
type UrlSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	Urls    []Url    `xml:"url"`
}
type Url struct {
	Loc, LastMod string
}

// --- RSS Structs (NEW) ---
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
	PubDate     string `xml:"pubDate"` // Format: Mon, 02 Jan 2006 15:04:05 GMT
	Guid        string `xml:"guid"`
}

// --- AST Transformer ---
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
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	}
	if _, isImage := n.(*ast.Image); isImage {
		n.SetAttribute([]byte("loading"), []byte("lazy"))
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

// --- Main Execution ---
func main() {
	baseUrlFlag := flag.String("baseurl", "", "Base URL")
	compressFlag := flag.Bool("compress", false, "Enable image compression")
	flag.Parse()
	BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	CompressImages = *compressFlag

	// Generate Build Version Timestamp
	currentBuildVersion := time.Now().Unix()

	fmt.Printf("🔨 Building site... (Version: %d)\n", currentBuildVersion)

	// 1. Check Global Template Timestamp
	layoutInfo, err := os.Stat("templates/layout.html")
	if err != nil {
		log.Fatal("Could not find templates/layout.html")
	}

	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		if layoutInfo.ModTime().After(indexInfo.ModTime()) {
			fmt.Println("⚡ Template changed. Forcing full rebuild.")
			ForceRebuild = true
		}
	} else {
		ForceRebuild = true // First run
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(highlighting.WithStyle("nord")),
			passthrough.New(passthrough.Config{
				InlineDelimiters: []passthrough.Delimiters{{Open: "$", Close: "$"}, {Open: "\\(", Close: "\\)"}},
				BlockDelimiters:  []passthrough.Delimiters{{Open: "$$", Close: "$$"}, {Open: "\\[", Close: "\\]"}},
			}),
		),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(&URLTransformer{}, 100))),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	os.MkdirAll("public/tags", 0755)

	if _, err := os.Stat("static"); err == nil {
		copyDir("static", "public/static")
	}

	var allPosts []PostMetadata
	var pinnedPosts []PostMetadata
	tagMap := make(map[string][]PostMetadata)

	funcMap := template.FuncMap{"lower": strings.ToLower}
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html")
	if err != nil {
		log.Fatal(err)
	}

	// Walk Content
	err = filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}

		// Determine paths
		relPath, _ := filepath.Rel("content", path)

		// --- NEW: Force Lowercase URLs ---
		// Replaces .md with .html AND converts the whole string to lowercase
		htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

		destPath := filepath.Join("public", htmlRelPath)
		fullLink := BaseURL + "/" + htmlRelPath

		// --- INCREMENTAL CHECK ---
		skipRendering := false
		if !ForceRebuild {
			if destInfo, err := os.Stat(destPath); err == nil {
				if destInfo.ModTime().After(info.ModTime()) {
					skipRendering = true
				}
			}
		}

		source, _ := os.ReadFile(path)
		var buf bytes.Buffer
		context := parser.NewContext()
		if err := md.Convert(source, &buf, parser.WithContext(context)); err != nil {
			return err
		}

		metaData := meta.Get(context)

		wordCount := len(strings.Fields(string(source)))
		readTime := int(math.Ceil(float64(wordCount) / 120.0))

		isPinned := false
		if p, ok := metaData["pinned"].(bool); ok {
			isPinned = p
		}

		dateStr := getString(metaData, "date")
		dateObj, _ := time.Parse("2006-01-02", dateStr)

		post := PostMetadata{
			Title: getString(metaData, "title"), Link: fullLink,
			Description: getString(metaData, "description"), Tags: getSlice(metaData, "tags"),
			ReadingTime: readTime, Pinned: isPinned, DateObj: dateObj,
		}

		

		// --- SEO: Calculate Image Path ---
		imagePath := BaseURL + "/static/images/favicon.ico" // Default fallback
		if img, ok := metaData["image"].(string); ok {
			imagePath = BaseURL + img
		}

		if !skipRendering {
			fmt.Printf("   Rendering: %s\n", htmlRelPath)
			renderPage(tmpl, destPath, PageData{
				Title: post.Title, Description: post.Description, Content: template.HTML(buf.String()), Meta: metaData, BaseURL: BaseURL,
				BuildVersion: currentBuildVersion,
				Permalink:    fullLink,  // <--- Pass Link
				Image:        imagePath, // <--- Pass Image
			})
		}

        if strings.Contains(path, "404.md") {
            return nil
        }

        if isPinned {
			pinnedPosts = append(pinnedPosts, post)
		} else {
			allPosts = append(allPosts, post)
		}
		for _, t := range post.Tags {
			tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], post)
		}

		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	sortPosts(allPosts)
	sortPosts(pinnedPosts)
	for k := range tagMap {
		sortPosts(tagMap[k])
	}

	// Always regenerate Home
	homeContent := template.HTML("")
	if homeSrc, err := os.ReadFile("content/_index.md"); err == nil {
		var buf bytes.Buffer
		md.Convert(homeSrc, &buf, parser.WithContext(parser.NewContext()))
		homeContent = template.HTML(buf.String())
	}
	renderPage(tmpl, "public/index.html", PageData{
		Title: "Kush Blogs", Content: homeContent, IsIndex: true, Posts: allPosts, PinnedPosts: pinnedPosts, BaseURL: BaseURL,
		BuildVersion: currentBuildVersion,
		Permalink:    BaseURL + "/",                          // <--- Home Link
		Image:        BaseURL + "/static/images/favicon.ico", // <--- Home Image
	})

	// Always regenerate Tags
	var allTags []TagData
	for t, posts := range tagMap {
		allTags = append(allTags, TagData{Name: t, Count: len(posts), Link: fmt.Sprintf("%s/tags/%s.html", BaseURL, t)})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

	renderPage(tmpl, "public/tags/index.html", PageData{
		Title: "All Tags", IsTagsIndex: true, AllTags: allTags, BaseURL: BaseURL,
		BuildVersion: currentBuildVersion,
		Permalink:    BaseURL + "/tags/index.html",           // <--- Tags Index Link
		Image:        BaseURL + "/static/images/favicon.ico", // <--- Default Image
	})

	for t, posts := range tagMap {
		renderPage(tmpl, fmt.Sprintf("public/tags/%s.html", t), PageData{
			Title: "#" + t, IsIndex: true, Posts: posts, BaseURL: BaseURL,
			BuildVersion: currentBuildVersion,
			Permalink:    fmt.Sprintf("%s/tags/%s.html", BaseURL, t), // <--- Specific Tag Link
			Image:        BaseURL + "/static/images/favicon.ico",     // <--- Default Image
		})
	}

	// --- GENERATORS ---
	allContent := append(allPosts, pinnedPosts...)
	generateSitemap(allContent, tagMap)
	generateRSS(allContent) // <--- NEW: Generate RSS

	// --- Google Verification File (Optional) ---
	// Uncomment if you need to upload a file for Google Search Console
	// copyFileStandard("googleXXXXXXXXXXXXXXXX.html", "public/googleXXXXXXXXXXXXXXXX.html")

	fmt.Println("✅ Build Complete.")
}

// --- Helpers ---

func sortPosts(posts []PostMetadata) {
	sort.Slice(posts, func(i, j int) bool { return posts[i].DateObj.After(posts[j].DateObj) })
}

func renderPage(tmpl *template.Template, path string, data PageData) {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()
	tmpl.Execute(f, data)
}

// Updated Sitemap Generator with LastMod
func generateSitemap(posts []PostMetadata, tags map[string][]PostMetadata) {
	var urls []Url
	urls = append(urls, Url{Loc: BaseURL + "/", LastMod: time.Now().Format("2006-01-02")})
	for _, p := range posts {
		urls = append(urls, Url{
			Loc:     p.Link,
			LastMod: p.DateObj.Format("2006-01-02"), // <--- Now includes Date
		})
	}
	for t := range tags {
		urls = append(urls, Url{Loc: fmt.Sprintf("%s/tags/%s.html", BaseURL, t)})
	}
	output, _ := xml.MarshalIndent(UrlSet{Urls: urls}, "  ", "    ")
	os.WriteFile("public/sitemap.xml", []byte(xml.Header+string(output)), 0644)
}

// NEW: RSS Generator
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

	output, _ := xml.MarshalIndent(rss, "", "  ")
	fullXML := []byte(xml.Header + string(output))
	os.WriteFile("public/rss.xml", fullXML, 0644)
	fmt.Println("📡 RSS Feed generated.")
}

func getString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
func getSlice(m map[string]interface{}, k string) []string {
	var res []string
	if v, ok := m[k]; ok {
		if l, ok := v.([]interface{}); ok {
			for _, i := range l {
				res = append(res, fmt.Sprintf("%v", i))
			}
		}
	}
	return res
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		if destInfo, err := os.Stat(destPath); err == nil {
			if destInfo.ModTime().After(info.ModTime()) {
				return nil
			}
		}

		ext := strings.ToLower(filepath.Ext(path))
		if (ext == ".jpg" || ext == ".jpeg" || ext == ".png") && CompressImages {
			fmt.Printf("Compressing: %s\n", relPath)
			return processImage(path, destPath, ext)
		}
		return copyFileStandard(path, destPath)
	})
}

func processImage(srcPath, dstPath, ext string) error {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return copyFileStandard(srcPath, dstPath)
	}
	if src.Bounds().Dx() > 1200 {
		src = imaging.Resize(src, 1200, 0, imaging.Lanczos)
	}
	if ext == ".png" {
		return imaging.Save(src, dstPath, imaging.PNGCompressionLevel(png.BestCompression))
	}
	return imaging.Save(src, dstPath, imaging.JPEGQuality(75))
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