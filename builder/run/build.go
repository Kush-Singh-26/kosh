package run

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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

	b.mu.Lock()
	b.buildCache.Dependencies = models.DependencyGraph{
		Tags:      make(map[string][]string),
		Templates: make(map[string][]string),
		Assets:    make(map[string][]string),
	}
	b.mu.Unlock()

	if b.buildCache.TemplateModTimes == nil {
		b.buildCache.TemplateModTimes = make(map[string]time.Time)
	}

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

	for _, dep := range globalDependencies {
		if info, err := os.Stat(dep); err == nil {
			b.buildCache.TemplateModTimes[dep] = info.ModTime()
		}
	}

	b.cfg.ForceRebuild = false
	b.md = mdParser.New(cfg.BaseURL, b.native, b.buildCache.DiagramCache, &b.mu)

	b.DestFs.MkdirAll("public/tags", 0755)
	b.DestFs.MkdirAll("public/static/images/cards", 0755)
	b.DestFs.MkdirAll("public/sitemap", 0755)

	// 2. Static Assets
	var staticWg sync.WaitGroup
	staticWg.Add(1)
	go func() {
		defer staticWg.Done()
		b.copyStaticAndBuildAssets()
	}()
	utils.WriteFileVFS(b.DestFs, "public/.nojekyll", []byte(""))
	staticWg.Wait()

	if len(affectedPosts) > 0 {
		b.mu.Lock()
		for _, postPath := range affectedPosts {
			delete(b.buildCache.Posts, utils.NormalizeCacheKey(postPath))
		}
		b.mu.Unlock()
	}

	// 3. Process Content (Posts)
	var (
		allPosts, pinnedPosts []models.PostMetadata
		tagMap                map[string][]models.PostMetadata
		indexedPosts          []models.IndexedPost
		anyPostChanged        bool
		has404                bool
	)

	if !shouldForce && len(affectedPosts) == 0 && len(globalDependencies) > 0 {
		// Template-only change detection logic
		isTemplateOnly := true
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

		if isTemplateOnly && lastBuildTime.Unix() > 0 && len(b.buildCache.Posts) > 0 {
			fmt.Println("🚀 Template-only change detected. Fast-tracking rebuild...")
			b.renderCachedPosts()

			// Hydrate data for global pages from cache
			tagMap = make(map[string][]models.PostMetadata)
			for _, cp := range b.buildCache.Posts {
				if cp.Metadata.Pinned {
					pinnedPosts = append(pinnedPosts, cp.Metadata)
				} else {
					allPosts = append(allPosts, cp.Metadata)
				}
				for _, t := range cp.Metadata.Tags {
					tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], cp.Metadata)
				}
				indexedPosts = append(indexedPosts, models.IndexedPost{Record: cp.SearchRecord, WordFreqs: cp.WordFreqs, DocLen: cp.DocLen})
			}
			utils.SortPosts(allPosts)
			utils.SortPosts(pinnedPosts)
			anyPostChanged = true // Trigger global page re-render
		} else {
			allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404 = b.processPosts(shouldForce, forceSocialRebuild)
		}
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
