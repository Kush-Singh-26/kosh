package run

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"my-ssg/builder/cache"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/utils"
)

// Build executes a single build pass
func (b *Builder) Build() {
	cfg := b.cfg
	numWorkers := runtime.NumCPU()
	fmt.Printf("🔨 Building site... (Version: %d) | Parallel Workers: %d\n", cfg.BuildVersion, numWorkers)

	// 1. Setup & Cache Invalidation
	b.checkWasmUpdate()

	globalDependencies := []string{
		filepath.Join(cfg.TemplateDir, "layout.html"),
		filepath.Join(cfg.TemplateDir, "index.html"),
		filepath.Join(cfg.TemplateDir, "404.html"),
		filepath.Join(cfg.TemplateDir, "graph.html"),
		filepath.Join(cfg.StaticDir, "css/layout.css"),
		filepath.Join(cfg.StaticDir, "css/theme.css"),
		"kosh.yaml",
		"builder/generators/pwa.go",
	}
	forceSocialRebuild := false
	shouldForce := b.cfg.ForceRebuild
	var affectedPosts []string
	var lastBuildTime time.Time

	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		lastBuildTime = indexInfo.ModTime()
		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil && info.ModTime().After(lastBuildTime) {
				fmt.Printf("⚡ Global change detected in [%s].\n", dep)
				affected := b.invalidateForTemplate(dep)
				if affected != nil {
					affectedPosts = append(affectedPosts, affected...)
				} else {
					shouldForce = true
				}
			}
		}
		if info, err := os.Stat("builder/generators/social.go"); err == nil && info.ModTime().After(lastBuildTime) {
			forceSocialRebuild = true
		}
	} else {
		shouldForce = true
		forceSocialRebuild = true
	}

	b.cfg.ForceRebuild = false

	var diagramCache map[string]string
	if b.diagramAdapter != nil {
		diagramCache = b.diagramAdapter.AsMap()
	} else {
		diagramCache = make(map[string]string)
	}
	b.md = mdParser.New(cfg.BaseURL, b.native, diagramCache, &b.mu)

	_ = b.DestFs.MkdirAll("public/tags", 0755)
	_ = b.DestFs.MkdirAll("public/static/images/cards", 0755)
	_ = b.DestFs.MkdirAll("public/sitemap", 0755)

	// 2. Static Assets
	var staticWg sync.WaitGroup
	staticWg.Add(1)
	go func() {
		defer staticWg.Done()
		b.copyStaticAndBuildAssets()
	}()
	_ = utils.WriteFileVFS(b.DestFs, "public/.nojekyll", []byte(""))
	staticWg.Wait()

	if len(affectedPosts) > 0 && b.cacheManager != nil {
		for _, postPath := range affectedPosts {
			relPath, _ := filepath.Rel("content", postPath)
			// Need PostID to delete.
			// invalidateForTemplate returns paths.
			// We can generate ID from path (empty UUID).
			postID := cache.GeneratePostID("", relPath)
			_ = b.cacheManager.DeletePost(postID)
		}
	}

	// 3. Process Content (Posts)
	var (
		allPosts, pinnedPosts []models.PostMetadata
		tagMap                map[string][]models.PostMetadata
		indexedPosts          []models.IndexedPost
		anyPostChanged        bool
		has404                bool
	)

	// Template-only change detection logic
	isTemplateOnly := true
	if shouldForce || len(affectedPosts) > 0 {
		isTemplateOnly = false
	} else if len(globalDependencies) > 0 {
		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil {
				if info.ModTime().After(lastBuildTime) {
					// Check if it's a template
					if !strings.HasSuffix(dep, ".html") && !strings.HasSuffix(dep, ".css") {
						isTemplateOnly = false
						break
					}
				}
			}
		}
	} else {
		isTemplateOnly = false
	}

	cachedCount := 0
	if b.cacheManager != nil {
		if stats, err := b.cacheManager.Stats(); err == nil {
			cachedCount = stats.TotalPosts
		}
	}

	if isTemplateOnly && lastBuildTime.Unix() > 0 && cachedCount > 0 {
		fmt.Println("🚀 Template-only change detected. Fast-tracking rebuild...")
		b.renderCachedPosts()

		// Hydrate data for global pages from cache
		tagMap = make(map[string][]models.PostMetadata)
		ids, _ := b.cacheManager.ListAllPosts()

		for _, id := range ids {
			cached, err := b.cacheManager.GetPostByID(id)
			if err != nil || cached == nil {
				continue
			}

			// Reconstruct models.PostMetadata
			post := models.PostMetadata{
				Title:       cached.Title,
				Link:        cached.Link,
				Description: cached.Description,
				Tags:        cached.Tags,
				ReadingTime: cached.ReadingTime,
				Pinned:      cached.Pinned,
				Draft:       cached.Draft,
				DateObj:     cached.Date,
				HasMath:     cached.HasMath,
				HasMermaid:  cached.HasMermaid,
			}

			if post.Pinned {
				pinnedPosts = append(pinnedPosts, post)
			} else {
				allPosts = append(allPosts, post)
			}
			for _, t := range post.Tags {
				tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], post)
			}

			// Indexed Posts
			searchMeta, err := b.cacheManager.GetSearchRecord(id)
			if err == nil && searchMeta != nil {
				// Reconstruct PostRecord
				rec := models.PostRecord{
					Title:       searchMeta.Title,
					Link:        cached.Link, // From PostMeta
					Description: cached.Description,
					Tags:        cached.Tags,
					Content:     searchMeta.Content, // Needs Content field added earlier
				}
				rec.ID = len(indexedPosts) // Assign ID sequentially

				indexedPosts = append(indexedPosts, models.IndexedPost{
					Record:    rec,
					WordFreqs: searchMeta.BM25Data,
					DocLen:    searchMeta.DocLen,
				})
			}
		}

		utils.SortPosts(allPosts)
		utils.SortPosts(pinnedPosts)
		anyPostChanged = true
	} else {
		allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404 = b.processPosts(shouldForce, forceSocialRebuild)
	}

	// 4. Generate Global Pages
	if shouldForce || anyPostChanged {
		b.renderPagination(allPosts, pinnedPosts)
	}

	if !has404 {
		b.rnd.Render404("public/404.html", models.PageData{
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Config:       cfg,
			TabTitle:     "404 - Page Not Found | " + cfg.Title,
		})
	}

	if shouldForce || anyPostChanged || forceSocialRebuild {
		b.renderTags(tagMap, forceSocialRebuild)
	}

	if shouldForce || anyPostChanged {
		b.rnd.RenderGraph("public/graph.html", models.PageData{
			Title:        "Graph View",
			TabTitle:     "Knowledge Graph | " + cfg.Title,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Config:       cfg,
		})
		allContent := append(allPosts, pinnedPosts...)
		b.generateMetadata(allContent, tagMap, indexedPosts, shouldForce)
	}

	// 5. PWA & Sync
	b.generatePWA(shouldForce)

	if err := utils.SyncVFS(b.DestFs, "public", b.rnd.GetRenderedFiles()); err != nil {
		fmt.Printf("❌ Failed to sync VFS to disk: %v\n", err)
	}
	b.rnd.ClearRenderedFiles()

	fmt.Println("✅ Build Complete.")
}
