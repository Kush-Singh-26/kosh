package run

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/config"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer"
	"my-ssg/builder/utils"
)

// Run executes the main build logic
func Run(args []string) {
	// Pass the arguments to the config loader so flags like -compress work correctly
	cfg := config.Load(args)

	// Use all available CPU cores
	numWorkers := runtime.NumCPU()
	fmt.Printf("🔨 Building site... (Version: %d) | Parallel Workers: %d\n", cfg.BuildVersion, numWorkers)

	socialCardCache, cacheErr := utils.LoadSocialCardCache("public/.social-card-cache.json")
	if cacheErr != nil {
		fmt.Printf("Warning: Failed to load social card cache: %v\n", cacheErr)
		socialCardCache = utils.NewSocialCardCache()
	}
	defer func() {
		if saveErr := utils.SaveSocialCardCache("public/.social-card-cache.json", socialCardCache); saveErr != nil {
			fmt.Printf("Warning: Failed to save social card cache: %v\n", saveErr)
		}
	}()

	utils.InitMinifier()

	// Check dependencies for force rebuild
	globalDependencies := []string{"templates/layout.html", "templates/index.html", "templates/404.html", "static/css/layout.css", "static/css/theme.css"}
	forceSocialRebuild := false

	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		lastBuildTime := indexInfo.ModTime()

		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil && info.ModTime().After(lastBuildTime) {
				fmt.Printf("⚡ Global change detected in [%s]. Forcing full rebuild.\n", dep)
				cfg.ForceRebuild = true
				break
			}
		}

		if info, err := os.Stat("builder/generators/social.go"); err == nil && info.ModTime().After(lastBuildTime) {
			fmt.Println("⚡ Social generator change detected. Forcing social card rebuild.")
			forceSocialRebuild = true
		}
	} else {
		cfg.ForceRebuild = true
		forceSocialRebuild = true
	}

	md := mdParser.New(cfg.BaseURL)
	rnd := renderer.New(cfg.CompressImages)

	os.MkdirAll("public/tags", 0755)
	os.MkdirAll("public/static/images/cards", 0755)
	os.MkdirAll("public/sitemap", 0755)

	if _, err := os.Stat("static"); err == nil {
		utils.CopyDir("static", "public/static", cfg.CompressImages)
	}

	// Process Assets (CSS/JS minification & hashing)
	assets, assetErr := utils.ProcessAssets("static", "public/static")
	if assetErr != nil {
		fmt.Printf("⚠️ Failed to process assets: %v\n", assetErr)
	} else {
		fmt.Printf("🎨 Processed %d assets\n", len(assets))
	}
	rnd.SetAssets(assets)

	if err := os.WriteFile("public/.nojekyll", []byte(""), 0644); err != nil {
		fmt.Printf("⚠️ Failed to create .nojekyll: %v\n", err)
	}

	fontsDir := "builder/assets/fonts"
	faviconPath := "static/images/favicon.png"

	// --- PARALLELIZATION START ---

	// 1. Shared State & Mutex
	var (
		allPosts      []models.PostMetadata
		pinnedPosts   []models.PostMetadata
		searchRecords []models.PostRecord
		tagMap        = make(map[string][]models.PostMetadata)
		has404        bool
		mu            sync.Mutex // Protects allPosts, pinnedPosts, tagMap, socialCardCache, has404, searchRecords
	)

	// 2. Collect all files first
	var filesToProcess []string
	err := filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}
		if strings.Contains(path, "404.md") {
			// Quick lock to update bool
			mu.Lock()
			has404 = true
			mu.Unlock()
			return nil
		}
		filesToProcess = append(filesToProcess, path)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. Process files concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, numWorkers) // Semaphore to limit concurrency

	for _, path := range filesToProcess {
		wg.Add(1)
		sem <- struct{}{} // Acquire token

		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }() // Release token

			// --- File Processing Logic (Moved inside goroutine) ---
			info, _ := os.Stat(path)
			relPath, _ := filepath.Rel("content", path)
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
			relPathNoExt := strings.TrimSuffix(htmlRelPath, ".html")

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

			// To extract plain text, we need the AST
			reader := text.NewReader(source)
			docNode := md.Parser().Parse(reader, parser.WithContext(context))
			plainText := mdParser.ExtractPlainText(docNode, source)

			// Reset reader for conversion (or we could just use the docNode with renderer)
			if err := md.Renderer().Render(&buf, source, docNode); err != nil {
				log.Printf("Error rendering %s: %v", path, err)
				return
			}

			htmlContent := buf.String()
			if cfg.CompressImages {
				htmlContent = utils.ReplaceToWebP(htmlContent)
			}

			metaData := meta.Get(context)

			isDraft, _ := metaData["draft"].(bool)
			if isDraft {
				fmt.Printf("⏩ Skipping draft: %s\n", relPath)
				return
			}

			wordCount := len(strings.Fields(string(source)))
			readTime := int(math.Ceil(float64(wordCount) / 120.0))
			isPinned, _ := metaData["pinned"].(bool)
			dateStr := utils.GetString(metaData, "date")
			dateObj, _ := time.Parse("2006-01-02", dateStr)
			hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")
			toc := mdParser.GetTOC(context)
			// Social Card Logic
			cardRelPath := relPathNoExt + ".webp"
			cardDestPath := filepath.Join("public", "static", "images", "cards", cardRelPath)
			os.MkdirAll(filepath.Dir(cardDestPath), 0755)

			genCard := false

			// Lock for Cache Read
			mu.Lock()
			cachedHash := socialCardCache.Hashes[relPath]
			mu.Unlock()

			if forceSocialRebuild {
				genCard = true
			} else {
				_, cardExists := os.Stat(cardDestPath)
				if os.IsNotExist(cardExists) {
					genCard = true
				} else {
					frontmatterHash, hashErr := utils.GetFrontmatterHash(metaData)
					if hashErr != nil {
						genCard = true
					} else {
						if cachedHash != frontmatterHash {
							genCard = true
						}
					}
				}
			}

			if genCard {
				fmt.Printf("   🖼️  Generating Social Card: %s\n", cardRelPath)
				err := generators.GenerateSocialCard(
					utils.GetString(metaData, "title"),
					utils.GetString(metaData, "description"),
					dateStr,
					cardDestPath,
					faviconPath,
					fontsDir,
				)
				if err != nil {
					fmt.Printf("      ⚠️ Failed to generate card: %v\n", err)
				} else {
					if frontmatterHash, hashErr := utils.GetFrontmatterHash(metaData); hashErr == nil {
						// Lock for Cache Write
						mu.Lock()
						socialCardCache.Hashes[relPath] = frontmatterHash
						mu.Unlock()
					}
				}
			} else {
				if frontmatterHash, hashErr := utils.GetFrontmatterHash(metaData); hashErr == nil {
					// Lock for Cache Write
					mu.Lock()
					socialCardCache.Hashes[relPath] = frontmatterHash
					mu.Unlock()
				}
			}

			imagePath := cfg.BaseURL + "/static/images/cards/" + cardRelPath
			if img, ok := metaData["image"].(string); ok {
				if cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = cfg.BaseURL + img
			}

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
					TOC:          toc,
				})
			}

			// Lock for Shared Data Write
			mu.Lock()
			if isPinned {
				pinnedPosts = append(pinnedPosts, post)
			} else {
				allPosts = append(allPosts, post)
			}
			for _, t := range post.Tags {
				key := strings.ToLower(strings.TrimSpace(t))
				tagMap[key] = append(tagMap[key], post)
			}
			searchRecords = append(searchRecords, models.PostRecord{
				ID:          len(searchRecords),
				Title:       post.Title,
				Link:        htmlRelPath,
				Description: post.Description,
				Tags:        post.Tags,
				Content:     plainText,
			})
			mu.Unlock()

		}(path)
	}

	wg.Wait()
	// --- PARALLELIZATION END ---

	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	// --- HOME CARD GENERATION START ---
	homeCardPath := "public/static/images/cards/home.webp"
	genHomeCard := false
	if forceSocialRebuild {
		genHomeCard = true
	} else {
		if _, err := os.Stat(homeCardPath); os.IsNotExist(err) {
			genHomeCard = true
		}
	}

	if genHomeCard {
		fmt.Println("   🖼️  Generating Home Social Card...")
		err := generators.GenerateSocialCard(
			"Kush Blogs",
			"A personal archive of learning, coding, and building. Documenting the journey through Mathematics and AI.",
			"",
			homeCardPath,
			faviconPath,
			fontsDir,
		)
		if err != nil {
			fmt.Printf("      ⚠️ Failed to generate home card: %v\n", err)
		}
	}
	// --- HOME CARD GENERATION END ---

	// --- PAGINATION START ---
	postsPerPage := cfg.PostsPerPage
	totalPages := int(math.Ceil(float64(len(allPosts)) / float64(postsPerPage)))

	if totalPages == 0 {
		totalPages = 1
	}

	for i := 1; i <= totalPages; i++ {
		start := (i - 1) * postsPerPage
		end := start + postsPerPage
		if end > len(allPosts) {
			end = len(allPosts)
		}

		pagePosts := allPosts[start:end]

		destPath := "public/index.html"
		permalink := cfg.BaseURL + "/"
		if i > 1 {
			destPath = fmt.Sprintf("public/page/%d/index.html", i)
			permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, i)
			os.MkdirAll(filepath.Dir(destPath), 0755)
		}

		paginator := models.Paginator{
			CurrentPage: i,
			TotalPages:  totalPages,
			HasPrev:     i > 1,
			HasNext:     i < totalPages,
			FirstURL:    cfg.BaseURL + "/#latest",
			LastURL:     fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages),
		}

		if i > 2 {
			paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i-1)
		} else if i == 2 {
			paginator.PrevURL = cfg.BaseURL + "/#latest"
		}

		if i < totalPages {
			paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i+1)
		}

		// Only show pinned posts on the first page
		var currentPinnedPosts []models.PostMetadata
		if i == 1 {
			currentPinnedPosts = pinnedPosts
		}

		rnd.RenderIndex(destPath, models.PageData{
			Title:        "Kush Blogs",
			Posts:        pagePosts,
			PinnedPosts:  currentPinnedPosts,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			TabTitle:     "Kush Blogs",
			Description:  "I write about machine learning, deep learning and lately more about NLP.",
			Permalink:    permalink,
			Image:        cfg.BaseURL + "/static/images/cards/home.webp",
			Paginator:    paginator,
		})
	}
	// --- PAGINATION END ---

	if !has404 {
		dest404 := "public/404.html"
		src404 := "templates/404.html"

		shouldBuild404 := false
		if cfg.ForceRebuild {
			shouldBuild404 = true
		} else {
			infoDest, errDest := os.Stat(dest404)
			infoSrc, errSrc := os.Stat(src404)

			if os.IsNotExist(errDest) {
				shouldBuild404 = true
			} else if errSrc == nil && infoSrc.ModTime().After(infoDest.ModTime()) {
				shouldBuild404 = true
			}
		}

		if shouldBuild404 {
			rnd.Render404(dest404, models.PageData{
				BaseURL:      cfg.BaseURL,
				BuildVersion: cfg.BuildVersion,
			})
			fmt.Println("📄 404 page rendered.")
		}
	}

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

	rnd.RenderGraph("public/graph.html", models.PageData{
		Title:        "Graph View",
		TabTitle:     "Knowledge Graph | Kush Blogs",
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
	})

	allContent := append(allPosts, pinnedPosts...)
	generators.GenerateSitemap(cfg.BaseURL, allContent, tagMap)
	generators.GenerateRSS(cfg.BaseURL, allContent)
	if err := generators.GenerateSearchIndex("public", searchRecords); err != nil {
		fmt.Printf("⚠️ Failed to generate search index: %v\n", err)
	}

	graphHash, graphHashErr := utils.GetGraphHash(allContent)
	genGraph := cfg.ForceRebuild
	if !genGraph && graphHashErr == nil {
		if socialCardCache.GraphHash != graphHash {
			genGraph = true
		}
	}

	if genGraph {
		generators.GenerateGraph(cfg.BaseURL, allContent)
		if graphHashErr == nil {
			socialCardCache.GraphHash = graphHash
		}
		fmt.Println("🕸️  Knowledge Graph regenerated.")
	}

	// --- PWA GENERATION START ---
	if err := generators.GenerateSW("public", cfg.BuildVersion, cfg.ForceRebuild); err != nil {
		fmt.Printf("⚠️ Failed to generate Service Worker: %v\n", err)
	}

	if err := generators.GeneratePWAIcons(faviconPath, "public/static/images"); err != nil {
		fmt.Printf("⚠️ Failed to generate PWA icons: %v\n", err)
	}
	// --- PWA GENERATION END ---

	fmt.Println("✅ Build Complete.")
}
