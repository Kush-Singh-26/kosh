package run

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

func (b *Builder) renderPagination(ctx context.Context, allPosts, pinnedPosts []models.PostMetadata, force bool) error {
	cfg := b.cfg

	// Generate Home Social Card
	homeCardPath := filepath.Join(b.cfg.OutputDir, "static/images/cards/home.webp")
	desc := cfg.Description
	if len(desc) > 100 {
		desc = desc[:97] + "..."
	}
	b.provideSocialCard(homeCardPath, "home", cfg.Title, desc, "Latest Posts", force)

	// For docs theme with versions, filter to only latest version posts for hub page
	latestPosts := allPosts
	if len(cfg.Versions) > 0 {
		// Find the latest version name
		var latestVersion string
		for _, v := range cfg.Versions {
			if v.IsLatest {
				latestVersion = v.Name
				break
			}
		}
		if latestVersion != "" {
			latestPosts = make([]models.PostMetadata, 0, len(allPosts)/len(cfg.Versions))
			for _, p := range allPosts {
				if p.Version == latestVersion {
					latestPosts = append(latestPosts, p)
				}
			}
		}
	}

	if cfg.PostsPerPage <= 0 {
		cfg.PostsPerPage = 10 // Safe default
	}
	totalPages := int(math.Ceil(float64(len(latestPosts)) / float64(cfg.PostsPerPage)))
	if totalPages == 0 {
		totalPages = 1
	}

	// Build SiteTree once before the loop (optimization: avoids recalculating for each page)
	siteTree := utils.BuildSiteTree(latestPosts, "")

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	for i := 1; i <= totalPages; i++ {
		pageIdx := i
		g.Go(func() error {
			start, end := (pageIdx-1)*cfg.PostsPerPage, pageIdx*cfg.PostsPerPage
			if end > len(latestPosts) {
				end = len(latestPosts)
			}
			pagePosts := latestPosts[start:end]
			destPath, permalink := filepath.Join(b.cfg.OutputDir, "index.html"), cfg.BaseURL+"/"
			if pageIdx > 1 {
				destPath = filepath.Join(b.cfg.OutputDir, fmt.Sprintf("page/%d/index.html", pageIdx))
				permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, pageIdx)
				_ = b.DestFs.MkdirAll(filepath.Dir(destPath), 0755)
			}
			paginator := models.Paginator{
				CurrentPage: pageIdx,
				TotalPages:  totalPages,
				HasPrev:     pageIdx > 1,
				HasNext:     pageIdx < totalPages,
				FirstURL:    cfg.BaseURL + "/#latest",
				LastURL:     fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages),
			}
			if pageIdx > 2 {
				paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx-1)
			} else if pageIdx == 2 {
				paginator.PrevURL = cfg.BaseURL + "/#latest"
			}
			if pageIdx < totalPages {
				paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx+1)
			}
			var curPinned []models.PostMetadata
			if pageIdx == 1 {
				curPinned = pinnedPosts
			}

			// Calculate relative path from output directory
			relPath := "index.html"
			if pageIdx > 1 {
				relPath = fmt.Sprintf("page/%d/index.html", pageIdx)
			}

			b.renderService.RenderIndex(destPath, models.PageData{
				Title: cfg.Title, Posts: pagePosts, PinnedPosts: curPinned,
				BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title,
				Description: cfg.Description, Permalink: permalink, Image: cfg.BaseURL + "/static/images/cards/home.webp",
				Paginator: paginator, SiteTree: siteTree, Config: cfg, Versions: cfg.GetVersionsMetadata("", ""),
				RelativePrefix: utils.GetRelativePrefix(relPath),
			})
			return nil
		})
	}
	return g.Wait()
}

func (b *Builder) renderTags(ctx context.Context, tagMap map[string][]models.PostMetadata, forceSocialRebuild bool) error {
	var allTags []models.TagData
	for t, posts := range tagMap {
		slug := utils.Slugify(t)
		allTags = append(allTags, models.TagData{Name: t, Count: len(posts), Link: fmt.Sprintf("/tags/%s.html", slug)})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

	// Generate Tags Index Card
	tagsIndexCard := filepath.Join(b.cfg.OutputDir, "static/images/cards/tags/index.webp")
	b.provideSocialCard(tagsIndexCard, "tags/index", "All Topics", fmt.Sprintf("Browse all %d topics", len(tagMap)), "Topics", forceSocialRebuild)

	// Generate Tags Index
	// Force Weight: 0 so layout doesn't crash
	b.renderService.RenderPage(filepath.Join(b.cfg.OutputDir, "tags/index.html"), models.PageData{
		Title: "All Tags", IsTagsIndex: true, AllTags: allTags,
		BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
		Permalink: b.cfg.BaseURL + "/tags/index.html",
		Image:     b.cfg.BaseURL + "/static/images/cards/tags/index.webp",
		TabTitle:  "All Topics | " + b.cfg.Title, Config: b.cfg,
		Weight:         0, // Fix for docs theme layout
		RelativePrefix: "../",
	})

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	for t, posts := range tagMap {
		tagName := t
		tagPosts := posts
		g.Go(func() error {
			slug := utils.Slugify(tagName)
			// Generate Tag Card
			tagCard := filepath.Join(b.cfg.OutputDir, fmt.Sprintf("static/images/cards/tags/%s.webp", slug))
			b.provideSocialCard(tagCard, "tags/"+slug, "#"+tagName, fmt.Sprintf("%d posts about %s", len(tagPosts), tagName), "Topic", forceSocialRebuild)

			utils.SortPosts(tagPosts)
			b.renderService.RenderPage(filepath.Join(b.cfg.OutputDir, fmt.Sprintf("tags/%s.html", slug)), models.PageData{
				Title: "#" + tagName, IsIndex: true, Posts: tagPosts,
				BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
				Permalink: fmt.Sprintf("%s/tags/%s.html", b.cfg.BaseURL, slug),
				Image:     fmt.Sprintf("%s/static/images/cards/tags/%s.webp", b.cfg.BaseURL, slug),
				TabTitle:  "#" + tagName + " | " + b.cfg.Title, Config: b.cfg,
				Weight:         0, // Fix for docs theme layout
				RelativePrefix: "../",
			})
			return nil
		})
	}
	return g.Wait()
}

// provideSocialCard ensures a social card exists in the VFS, using cache if possible
func (b *Builder) provideSocialCard(destPath string, cacheKey string, title, description, badge string, force bool) {
	// 1. Calculate Hash (must match post_social.go logic for consistency)
	cardContent := fmt.Sprintf("%s|%s", title, description)
	currentHash := cache.HashString(cardContent)

	cachedCardPath := filepath.Join(b.cfg.CacheDir, "social-cards", currentHash+".webp")

	// 2. Check if we need to generate
	needsGen := force
	if !needsGen {
		if _, err := os.Stat(cachedCardPath); os.IsNotExist(err) {
			needsGen = true
		} else if b.cacheService != nil {
			storedHash, _ := b.cacheService.GetSocialCardHash(cacheKey)
			if storedHash != currentHash {
				needsGen = true
			}
		}
	}

	// 3. Ensure Dir in VFS
	_ = b.DestFs.MkdirAll(filepath.Dir(destPath), 0755)

	if needsGen {
		// Generate to disk (cache)
		_ = os.MkdirAll(filepath.Dir(cachedCardPath), 0755)
		faviconPath := b.getFaviconPath()
		err := generators.GenerateSocialCardToDisk(b.SourceFs, &b.cfg.SocialCards, b.cfg.Title, title, description, badge, cachedCardPath, faviconPath)
		if err != nil {
			b.logger.Warn("Failed to generate card", "path", destPath, "error", err)
			return
		}
		if b.cacheService != nil {
			_ = b.cacheService.SetSocialCardHash(cacheKey, currentHash)
		}
	}

	// 4. Copy from Cache (disk) to VFS
	data, err := os.ReadFile(cachedCardPath)
	if err == nil {
		_ = afero.WriteFile(b.DestFs, destPath, data, 0644)
		b.renderService.RegisterFile(destPath)
	}
}
