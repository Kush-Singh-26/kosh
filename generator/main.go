package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

type Meta struct {
	Title string `yaml:"title"`
	Date  string `yaml:"date"`
	Slug  string `yaml:"slug"`
}

type ManifestItem struct {
	File    string   `json:"file"`
	IsEntry bool     `json:"isEntry"`
	CSS     []string `json:"css"`
}

func readManifest(path string) (jsFiles []string, cssFiles []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("warn: could not read manifest.json at %s, proceeding without assets", path)
		return
	}
	var m map[string]ManifestItem
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("warn: could not parse manifest.json: %v", err)
		return
	}
	for _, item := range m {
		if item.IsEntry {
			jsFiles = append(jsFiles, item.File)
			cssFiles = append(cssFiles, item.CSS...)
		}
	}
	return
}

func parseMarkdown(path string) (Meta, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, nil, err
	}

	// FIX: Normalize Windows line endings (\r\n) to Unix endings (\n)
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))

	var meta Meta
	var body []byte

	if bytes.HasPrefix(b, []byte("---\n")) {
		parts := bytes.SplitN(b, []byte("\n---\n"), 2)
		if len(parts) == 2 {
			if err := yaml.Unmarshal(parts[0][4:], &meta); err != nil {
				return Meta{}, nil, err
			}
			body = parts[1]
		} else {
			body = b
		}
	} else {
		body = b
	}
	return meta, body, nil
}

func copyAssets(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

func main() {
	contentDir := flag.String("content", "content/posts", "Directory with markdown posts")
	templateDir := flag.String("templates", "templates", "Directory with HTML templates")
	assetsDir := flag.String("assets", "frontend/dist", "Directory with frontend assets")
	outDir := flag.String("out", "public", "Output directory for the site")
	base := flag.String("base", "/", "Base URL for links")
	flag.Parse()

	log.Println("Starting site generation...")
	os.RemoveAll(*outDir)
	os.MkdirAll(*outDir, 0o755)

	tmpl := template.Must(template.ParseGlob(filepath.Join(*templateDir, "*.html")))
	jsFiles, cssFiles := readManifest(filepath.Join(*assetsDir, "manifest.json"))
	log.Printf("Found assets: JS=%v, CSS=%v", jsFiles, cssFiles)

	md := goldmark.New()
	policy := bluemonday.UGCPolicy()

	var posts []Meta
	filepath.Walk(*contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		meta, body, err := parseMarkdown(path)
		if err != nil {
			log.Printf("error parsing markdown %s: %v", path, err)
			return nil
		}
		if meta.Slug == "" || meta.Title == "" {
			log.Printf("skipping %s: missing slug or title", path)
			return nil
		}
		var buf bytes.Buffer
		md.Convert(body, &buf)
		safeHTML := policy.SanitizeBytes(buf.Bytes())
		outPath := filepath.Join(*outDir, meta.Slug, "index.html")
		os.MkdirAll(filepath.Dir(outPath), 0o755)
		f, _ := os.Create(outPath)
		defer f.Close()
		data := map[string]any{
			"Title":   meta.Title,
			"Date":    meta.Date,
			"Content": template.HTML(safeHTML),
			"Base":    *base,
			"CSS":     cssFiles,
			"JS":      jsFiles,
		}
		tmpl.ExecuteTemplate(f, "post.html", data)
		log.Printf("Generated post: %s", outPath)
		posts = append(posts, meta)
		return nil
	})

	sort.Slice(posts, func(i, j int) bool {
		t1, _ := time.Parse("2006-01-02", posts[i].Date)
		t2, _ := time.Parse("2006-01-02", posts[j].Date)
		return t1.After(t2)
	})

	indexPath := filepath.Join(*outDir, "index.html")
	f, _ := os.Create(indexPath)
	defer f.Close()
	data := map[string]any{
		"Posts": posts,
		"Base":  *base,
		"CSS":   cssFiles,
		"JS":    jsFiles,
	}
	tmpl.ExecuteTemplate(f, "index.html", data)
	log.Printf("Generated homepage: %s", indexPath)

	if _, err := os.Stat(filepath.Join(*assetsDir, "assets")); !os.IsNotExist(err) {
		copyAssets(filepath.Join(*assetsDir), *outDir)
		log.Println("Copied assets.")
	}

	os.WriteFile(filepath.Join(*outDir, ".nojekyll"), []byte{}, 0o644)
	log.Println("Site generation complete.")
}