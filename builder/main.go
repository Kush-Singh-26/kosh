package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"image/png" // Needed for PNG compression constants
	"io"
	"io/fs"
	"log"
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

// --- Global Configuration ---
var BaseURL string

// --- Data Structures ---
type PostMetadata struct {
	Title       string
	Link        string
	Description string
	Date        string
	Tags        []string
}

type TagData struct {
	Name  string
	Count int
	Link  string
}

type PageData struct {
	Title       string
	Description string
	BaseURL     string
	Content     template.HTML
	Meta        map[string]interface{}
	IsIndex     bool
	IsTagsIndex bool
	Posts       []PostMetadata
	AllTags     []TagData
}

// --- Sitemap XML Structures ---
type UrlSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	Urls    []Url    `xml:"url"`
}
type Url struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// --- AST Transformer (Fixes URLs for Links AND Images) ---
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

	// 1. Handle External Links (New Tab) - Only for Links
	if strings.HasPrefix(href, "http") {
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	}

	// 2. Handle Internal BaseURL (For GitHub Pages)
	// If it starts with "/" and we have a BaseURL, prepend it.
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
	// 1. Parse Flags
	baseUrlFlag := flag.String("baseurl", "", "Base URL for the site (e.g. /my-repo)")
	flag.Parse()
	BaseURL = strings.TrimSuffix(*baseUrlFlag, "/") // Normalize: no trailing slash

	fmt.Printf("🔨 Building site with BaseURL: '%s' ...\n", BaseURL)

	// 2. Configure Goldmark
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
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&URLTransformer{}, 100),
			),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	// 3. Prepare Output Directory
	os.RemoveAll("public")
	os.MkdirAll("public/tags", 0755)
	
	// 4. Copy & Optimize Static Assets
	if _, err := os.Stat("static"); err == nil {
		fmt.Println("📂 Copying and optimizing static assets...")
		if err := copyDir("static", "public/static"); err != nil {
			log.Println("Error copying static:", err)
		}
	}

	// 5. Initialize Data & Templates
	var allPosts []PostMetadata
	tagMap := make(map[string][]PostMetadata)
	
	funcMap := template.FuncMap{"lower": strings.ToLower}
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html")
	if err != nil {
		log.Fatal("Template Parsing Error:", err)
	}

	// 6. Walk Content
	err = filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}

		source, _ := os.ReadFile(path)
		var buf bytes.Buffer
		context := parser.NewContext()
		if err := md.Convert(source, &buf, parser.WithContext(context)); err != nil {
			return err
		}

		metaData := meta.Get(context)
		relPath, _ := filepath.Rel("content", path)
		htmlRelPath := strings.Replace(relPath, ".md", ".html", 1)
		
		// The link stored in metadata should include the BaseURL
		fullLink := BaseURL + "/" + htmlRelPath

		post := PostMetadata{
			Title:       getString(metaData, "title"),
			Link:        fullLink,
			Description: getString(metaData, "description"),
			Date:        getString(metaData, "date"),
			Tags:        getSlice(metaData, "tags"),
		}
		allPosts = append(allPosts, post)

		for _, t := range post.Tags {
			tLower := strings.ToLower(strings.TrimSpace(t))
			tagMap[tLower] = append(tagMap[tLower], post)
		}

		renderPage(tmpl, "public/"+htmlRelPath, PageData{
			Title:   post.Title,
			Description: post.Description,
			Content: template.HTML(buf.String()),
			Meta:    metaData,
			BaseURL: BaseURL,
		})
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	// 7. Generate Custom Home Page
	homeContent := template.HTML("")
	homeDesc := ""
	if homeSrc, err := os.ReadFile("content/_index.md"); err == nil {
		var buf bytes.Buffer
		context := parser.NewContext()
		md.Convert(homeSrc, &buf, parser.WithContext(context))
		homeContent = template.HTML(buf.String())
		homeDesc = getString(meta.Get(context), "description")
	}
	
	renderPage(tmpl, "public/index.html", PageData{
		Title:       "Home",
		Description: homeDesc,
		Content:     homeContent,
		IsIndex:     true,
		Posts:       allPosts,
		BaseURL:     BaseURL,
	})

	// 8. Generate Tags Index
	var allTags []TagData
	for t, posts := range tagMap {
		allTags = append(allTags, TagData{
			Name: t,
			Count: len(posts),
			Link: fmt.Sprintf("%s/tags/%s.html", BaseURL, t),
		})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })
	
	renderPage(tmpl, "public/tags/index.html", PageData{
		Title:       "All Tags",
		IsTagsIndex: true,
		AllTags:     allTags,
		BaseURL:     BaseURL,
	})

	// 9. Generate Individual Tag Pages
	for t, posts := range tagMap {
		renderPage(tmpl, fmt.Sprintf("public/tags/%s.html", t), PageData{
			Title:   "#" + t,
			IsIndex: true,
			Posts:   posts,
			BaseURL: BaseURL,
		})
	}

	// 10. Generate Sitemap
	generateSitemap(allPosts, tagMap)

	fmt.Println("✅ Build Complete.")
}

// --- Helper Functions ---

func renderPage(tmpl *template.Template, path string, data PageData) {
	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		log.Printf("Error rendering %s: %v", path, err)
	}
}

func generateSitemap(posts []PostMetadata, tags map[string][]PostMetadata) {
	var urls []Url
	// Homepage
	urls = append(urls, Url{Loc: BaseURL + "/", LastMod: time.Now().Format("2006-01-02")})
	// Posts
	for _, p := range posts {
		urls = append(urls, Url{Loc: p.Link}) 
	}
	// Tags
	for t := range tags {
		urls = append(urls, Url{Loc: fmt.Sprintf("%s/tags/%s.html", BaseURL, t)})
	}
	output, _ := xml.MarshalIndent(UrlSet{Urls: urls}, "  ", "    ")
	os.WriteFile("public/sitemap.xml", []byte(xml.Header+string(output)), 0644)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok { return fmt.Sprintf("%v", v) }
	return ""
}

func getSlice(m map[string]interface{}, key string) []string {
	var res []string
	if v, ok := m[key]; ok {
		if list, ok := v.([]interface{}); ok {
			for _, item := range list { res = append(res, fmt.Sprintf("%v", item)) }
		}
	}
	return res
}

// --- Image Processing Helpers ---

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil { return err }
		
		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Incremental Build Check
		if destInfo, err := os.Stat(destPath); err == nil {
			if destInfo.ModTime().After(info.ModTime()) {
				return nil 
			}
		}

		// Process Images (Compress/Resize)
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			fmt.Printf("Processing Image: %s\n", relPath)
			return processImage(path, destPath, ext)
		}

		// Standard Copy for other files
		return copyFileStandard(path, destPath)
	})
}

func processImage(srcPath, dstPath, ext string) error {
	// 1. Open the image
	src, err := imaging.Open(srcPath)
	if err != nil {
		// Fallback to standard copy if imaging fails (e.g. unknown format)
		return copyFileStandard(srcPath, dstPath)
	}

	// 2. Resize if too big (Max Width 1200px, maintain aspect ratio)
	if src.Bounds().Dx() > 1200 {
		src = imaging.Resize(src, 1200, 0, imaging.Lanczos)
	}

	// 3. Save with compression
	if ext == ".png" {
		return imaging.Save(src, dstPath, imaging.PNGCompressionLevel(png.BestCompression))
	} else {
		// JPG
		return imaging.Save(src, dstPath, imaging.JPEGQuality(75))
	}
}

func copyFileStandard(src, dst string) error {
	s, err := os.Open(src)
	if err != nil { return err }
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil { return err }
	defer d.Close()
	_, err = io.Copy(d, s)
	return err
}