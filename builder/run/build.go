package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"my-ssg/builder/cache"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

// Build executes a single build pass
func (b *Builder) Build(ctx context.Context) error {
	// Check for cancellation early
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cfg := b.cfg
	// Build started - minimal logging

	// 1. Setup & Cache Invalidation
	var setupWg sync.WaitGroup
	setupWg.Add(1)
	go func() {
		defer setupWg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			b.checkWasmUpdate()
		}
	}()

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

	if indexInfo, err := os.Stat(filepath.Join(b.cfg.OutputDir, "index.html")); err == nil {
		lastBuildTime = indexInfo.ModTime()
		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil && info.ModTime().After(lastBuildTime) {
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
		// public/ is missing (e.g., after 'kosh clean').
		// We do NOT force a full rebuild. We use the cache to rehydrate files.
		// shouldForce stays false.
		// forceSocialRebuild stays false (rely on missing file checks).
	}

	b.cfg.ForceRebuild = false

	if err := b.DestFs.MkdirAll(filepath.Join(b.cfg.OutputDir, "tags"), 0755); err != nil {
		b.logger.Error("Failed to create tags directory", "error", err)
	}

	if err := b.DestFs.MkdirAll(filepath.Join(b.cfg.OutputDir, "static/images/cards"), 0755); err != nil {
		b.logger.Error("Failed to create static images cards directory", "error", err)
	}
	if err := b.DestFs.MkdirAll(filepath.Join(b.cfg.OutputDir, "sitemap"), 0755); err != nil {
		b.logger.Error("Failed to create sitemap directory", "error", err)
	}

	// 2. Static Assets
	var staticWg sync.WaitGroup
	staticWg.Add(1)
	go func() {
		defer staticWg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			b.copyStaticAndBuildAssets()
		}
	}()
	_ = utils.WriteFileVFS(b.DestFs, filepath.Join(b.cfg.OutputDir, ".nojekyll"), []byte(""))
	staticWg.Wait()

	if len(affectedPosts) > 0 && b.cacheService != nil {
		for _, postPath := range affectedPosts {
			relPath, _ := utils.SafeRel(b.cfg.ContentDir, postPath)
			// Need PostID to delete.
			// invalidateForTemplate returns paths.
			// We can generate ID from path (empty UUID).
			postID := cache.GeneratePostID("", relPath)
			_ = b.cacheService.DeletePost(postID)
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
	isTemplateOnly := false // Default to false to ensure content changes are detected

	if shouldForce || len(affectedPosts) > 0 {
		isTemplateOnly = false
	} else if len(globalDependencies) > 0 {
		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil {
				if info.ModTime().After(lastBuildTime) {
					// Check if it's a template
					if strings.HasSuffix(dep, ".html") || strings.HasSuffix(dep, ".css") {
						isTemplateOnly = true
					} else {
						isTemplateOnly = false
						break
					}
				}
			}
		}
	}

	cachedCount := 0
	if b.cacheService != nil {
		if stats, err := b.cacheService.Stats(); err == nil {
			cachedCount = stats.TotalPosts
		}
	}

	// Use fast path if:
	// 1. Template-only changes AND we have a valid lastBuildTime, OR
	// 2. Output is missing (cleaned) AND we have cached data
	outputMissing := lastBuildTime.IsZero()
	if isTemplateOnly && ((!lastBuildTime.IsZero()) || outputMissing) && cachedCount > 0 {
		fmt.Println("📝 Rehydrating from cache...")
		b.renderCachedPosts()

		// Hydrate data for global pages from cache
		tagMap = make(map[string][]models.PostMetadata)
		ids, _ := b.cacheService.ListAllPosts()

		// Batch fetch all posts and search records in single transactions (avoids N+1 queries)
		cachedPosts, _ := b.cacheService.GetPostsByIDs(ids)
		searchRecords, _ := b.cacheService.GetSearchRecords(ids)

		for _, id := range ids {
			cached, ok := cachedPosts[id]
			if !ok || cached == nil {
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
				Version:     cached.Version,
			}

			if post.Pinned {
				pinnedPosts = append(pinnedPosts, post)
			} else {
				allPosts = append(allPosts, post)
			}
			for _, t := range post.Tags {
				tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], post)
			}

			// Indexed Posts - use batch-fetched search records
			if searchMeta, ok := searchRecords[id]; ok && searchMeta != nil {
				// Reconstruct PostRecord with relative link (not full URL)
				relLink := strings.ToLower(strings.Replace(cached.Path, ".md", ".html", 1))
				rec := models.PostRecord{
					Title:       searchMeta.Title,
					Link:        relLink, // Use relative link, not cached.Link which includes baseURL
					Description: cached.Description,
					Tags:        cached.Tags,
					Content:     searchMeta.Content,
					Version:     cached.Version,
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
		fmt.Println("📝 Processing content...")
		allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404 = b.processPosts(shouldForce, forceSocialRebuild, outputMissing)
		fmt.Println("   ✅ Content processed.")
	}

	// 4. Generate Global Pages
	if shouldForce || anyPostChanged {
		fmt.Println("📄 Rendering pagination...")
		b.renderPagination(allPosts, pinnedPosts, shouldForce)
	}

	if !has404 {
		b.renderService.Render404(filepath.Join(b.cfg.OutputDir, "404.html"), models.PageData{
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Config:       cfg,
			TabTitle:     "404 - Page Not Found | " + cfg.Title,
		})
	}

	if shouldForce || anyPostChanged || forceSocialRebuild {
		fmt.Println("🏷️  Rendering tags...")
		b.renderTags(tagMap, forceSocialRebuild)
	}

	if shouldForce || anyPostChanged {
		fmt.Println("🕸️  Rendering graph and metadata...")
		b.renderService.RenderGraph(filepath.Join(b.cfg.OutputDir, "graph.html"), models.PageData{
			Title:        "Graph View",
			TabTitle:     "Knowledge Graph | " + cfg.Title,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Config:       cfg,
		})
		allContent := append(allPosts, pinnedPosts...)
		b.generateMetadata(allContent, tagMap, indexedPosts, shouldForce)
	}

	// 5. PWA (Run concurrently)
	if cfg.Features.Generators.PWA {
		setupWg.Add(1)
		go func() {
			defer setupWg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Println("📱 Generating PWA...")
				b.generatePWA(shouldForce)
			}
		}()
	}

	// Ensure setup tasks (WASM check + PWA) are complete
	setupWg.Wait()

	// Now sync VFS to disk (includes completed social cards)
	fmt.Println("💾 Syncing to disk...")
	if err := utils.SyncVFS(b.DestFs, b.cfg.OutputDir, b.renderService.GetRenderedFiles()); err != nil {
		b.logger.Error("Failed to sync VFS to disk", "error", err)
	}
	b.renderService.ClearRenderedFiles()

	// Build complete
	return nil
}

func (b *Builder) copyStaticAndBuildAssets() {
	if err := b.assetService.Build(context.Background()); err != nil {
		b.logger.Error("Failed to build assets", "error", err)
	}
}

func (b *Builder) processPosts(shouldForce, forceSocialRebuild, outputMissing bool) ([]models.PostMetadata, []models.PostMetadata, map[string][]models.PostMetadata, []models.IndexedPost, bool, bool) {
	result, err := b.postService.Process(context.Background(), shouldForce, forceSocialRebuild, outputMissing)
	if err != nil {
		b.logger.Error("Failed to process posts", "error", err)
		return nil, nil, nil, nil, false, false
	}
	return result.AllPosts, result.PinnedPosts, result.TagMap, result.IndexedPosts, result.AnyPostChanged, result.Has404
}

func (b *Builder) renderCachedPosts() {
	b.postService.RenderCachedPosts()
}
