package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark/parser"
	meta "github.com/yuin/goldmark-meta"

	"my-ssg/builder/config"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer"
	"my-ssg/builder/utils"
)

func main() {
	cfg := config.Load()

	fmt.Printf("🔨 Building site... (Version: %d)\n", cfg.BuildVersion)

	// Initialize Minifier
	utils.InitMinifier()

	// Check dependencies for force rebuild
	globalDependencies := []string{"templates/layout.html", "templates/index.html", "templates/404.html", "static/css/layout.css", "static/css/theme.css"}
	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		lastBuildTime := indexInfo.ModTime()
		for _, dep := range globalDependencies {
			info, err := os.Stat(dep)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastBuildTime) {
				fmt.Printf("⚡ Global change detected in [%s]. Forcing full rebuild.\n", dep)
				cfg.ForceRebuild = true
				break
			}
		}
	} else {
		cfg.ForceRebuild = true
	}

	// Initialize components
	md := mdParser.New(cfg.BaseURL)
	rnd := renderer.New(cfg.CompressImages)

	// Prepare directories
	os.MkdirAll("public/tags", 0755)
	if _, err := os.Stat("static"); err == nil {
		utils.CopyDir("static", "public/static", cfg.CompressImages)
	}

	var allPosts []models.PostMetadata
	var pinnedPosts []models.PostMetadata
	tagMap := make(map[string][]models.PostMetadata)
	var has404 bool

	// Walk Content - Skip _index.md as it's no longer used
	err := filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}
		
		// Check if this is the 404 page
		if strings.Contains(path, "404.md") {
			has404 = true
			return nil
		}
		
		relPath, _ := filepath.Rel("content", path)
		htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
		destPath := filepath.Join("public", htmlRelPath)
		fullLink := cfg.BaseURL + "/" + htmlRelPath

		skipRendering := false
		if !cfg.ForceRebuild {
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

		htmlContent := buf.String()
		if cfg.CompressImages {
			htmlContent = utils.ReplaceToWebP(htmlContent)
		}

		metaData := meta.Get(context)

		isDraft, _ := metaData["draft"].(bool)
		if isDraft {
			fmt.Printf("⏩ Skipping draft: %s\n", relPath)
			return nil
		}

		wordCount := len(strings.Fields(string(source)))
		readTime := int(math.Ceil(float64(wordCount) / 120.0))
		isPinned, _ := metaData["pinned"].(bool)
		dateStr := utils.GetString(metaData, "date")
		dateObj, _ := time.Parse("2006-01-02", dateStr)
		hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")

		post := models.PostMetadata{
			Title:       utils.GetString(metaData, "title"),
			Link:        fullLink,
			Description: utils.GetString(metaData, "description"),
			Tags:        utils.GetSlice(metaData, "tags"),
			ReadingTime: readTime,
			Pinned:      isPinned,
			DateObj:     dateObj,
			HasMath:     hasMath,
		}

		imagePath := cfg.BaseURL + "/static/images/favicon.webp"
		if img, ok := metaData["image"].(string); ok {
			if cfg.CompressImages && !strings.HasPrefix(img, "http") {
				ext := filepath.Ext(img)
				if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
					img = img[:len(img)-len(ext)] + ".webp"
				}
			}
			imagePath = cfg.BaseURL + img
		}

		if !skipRendering {
			fmt.Printf("   Rendering: %s\n", htmlRelPath)
			rnd.RenderPage(destPath, models.PageData{
				Title:        post.Title,
				Description:  post.Description,
				Content:      template.HTML(htmlContent),
				Meta:         metaData,
				BaseURL:      cfg.BaseURL,
				BuildVersion: cfg.BuildVersion,
				TabTitle:     post.Title + " | Kush Blogs",
				Permalink:    fullLink,
				Image:        imagePath,
				HasMath:      post.HasMath,
			})
		}

		if isPinned {
			pinnedPosts = append(pinnedPosts, post)
		} else {
			allPosts = append(allPosts, post)
		}
		for _, t := range post.Tags {
			key := strings.ToLower(strings.TrimSpace(t))
			tagMap[key] = append(tagMap[key], post)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	// Render Home using dedicated index template
	rnd.RenderIndex("public/index.html", models.PageData{
		Title:        "Kush Blogs",
		Posts:        allPosts,
		PinnedPosts:  pinnedPosts,
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
		TabTitle:     "Kush Blogs",
		Description:  "I write about machine learning, deep learning and lately more about NLP.",
		Permalink:    cfg.BaseURL + "/",
		Image:        cfg.BaseURL + "/static/images/favicon.webp",
	})

	// Render 404 page using dedicated template (if 404.md doesn't exist)
	if !has404 {
		rnd.Render404("public/404.html", models.PageData{
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
		})
		fmt.Println("📄 404 page rendered.")
	}

	// Render Tags Index
	var allTags []models.TagData
	for t, posts := range tagMap {
		allTags = append(allTags, models.TagData{
			Name:  t,
			Count: len(posts),
			Link:  fmt.Sprintf("%s/tags/%s.html", cfg.BaseURL, t),
		})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

	rnd.RenderPage("public/tags/index.html", models.PageData{
		Title:        "All Tags",
		IsTagsIndex:  true,
		AllTags:      allTags,
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
		Permalink:    cfg.BaseURL + "/tags/index.html",
		Image:        cfg.BaseURL + "/static/images/favicon.webp",
		TabTitle:     "Kush Blogs",
	})

	// Render Individual Tags
	for t, posts := range tagMap {
		utils.SortPosts(posts)
		rnd.RenderPage(fmt.Sprintf("public/tags/%s.html", t), models.PageData{
			Title:        "#" + t,
			IsIndex:      true,
			Posts:        posts,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Permalink:    fmt.Sprintf("%s/tags/%s.html", cfg.BaseURL, t),
			Image:        cfg.BaseURL + "/static/images/favicon.webp",
			TabTitle:     "Kush Blogs",
		})
	}

	// Render Graph Page
	rnd.RenderGraph("public/graph.html", models.PageData{
		Title:        "Graph View",
		TabTitle:     "Knowledge Graph | Kush Blogs",
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
	})
	fmt.Println("🕸️  Graph HTML rendered.")

	// Generators
	allContent := append(allPosts, pinnedPosts...)
	generators.GenerateSitemap(cfg.BaseURL, allContent, tagMap)
	generators.GenerateRSS(cfg.BaseURL, allContent)
	generators.GenerateGraph(cfg.BaseURL, allContent)

	fmt.Println("✅ Build Complete.")
}