package run

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func hashString(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func (b *Builder) renderPagination(allPosts, pinnedPosts []models.PostMetadata, force bool) {
	cfg := b.cfg

	// Generate Home Social Card
	homeCardPath := filepath.Join(b.cfg.OutputDir, "static/images/cards/home.webp")

	// Check content hash
	cardContent := cfg.Title + "|" + cfg.Description
	currentHash := hashString(cardContent)
	needsGen := false

	if _, err := os.Stat(homeCardPath); os.IsNotExist(err) || force {
		needsGen = true
	} else if b.cacheService != nil {
		cachedHash, _ := b.cacheService.GetSocialCardHash("home")
		if cachedHash != currentHash {
			needsGen = true
		}
	}

	if needsGen {
		_ = b.DestFs.MkdirAll(filepath.Dir(homeCardPath), 0755)
		faviconPath := ""
		if b.cfg.Logo != "" {
			faviconPath = b.cfg.Logo
		} else {
			faviconPath = filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
		}
		_ = os.MkdirAll(filepath.Dir(homeCardPath), 0755)

		desc := cfg.Description
		if len(desc) > 100 {
			desc = desc[:97] + "..."
		}

		err := generators.GenerateSocialCardToDisk(b.SourceFs, &b.cfg.SocialCards, b.cfg.Title, cfg.Title, desc, "Latest Posts", homeCardPath, faviconPath)
		if err != nil {
			b.logger.Warn("Failed to generate home card", "error", err)
		} else if b.cacheService != nil {
			_ = b.cacheService.SetSocialCardHash("home", currentHash)
		}
	}

	totalPages := int(math.Ceil(float64(len(allPosts)) / float64(cfg.PostsPerPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			start, end := (i-1)*cfg.PostsPerPage, i*cfg.PostsPerPage
			if end > len(allPosts) {
				end = len(allPosts)
			}
			pagePosts := allPosts[start:end]
			destPath, permalink := filepath.Join(b.cfg.OutputDir, "index.html"), cfg.BaseURL+"/"
			if i > 1 {
				destPath = filepath.Join(b.cfg.OutputDir, fmt.Sprintf("page/%d/index.html", i))
				permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, i)
				_ = b.DestFs.MkdirAll(filepath.Dir(destPath), 0755)
			}
			paginator := models.Paginator{CurrentPage: i, TotalPages: totalPages, HasPrev: i > 1, HasNext: i < totalPages, FirstURL: cfg.BaseURL + "/#latest", LastURL: fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages)}
			if i > 2 {
				paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i-1)
			} else if i == 2 {
				paginator.PrevURL = cfg.BaseURL + "/#latest"
			}
			if i < totalPages {
				paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i+1)
			}
			var curPinned []models.PostMetadata
			if i == 1 {
				curPinned = pinnedPosts
			}

			// Build SiteTree for docs theme navigation (Full tree for the home page)
			// Root index should show all root level docs
			siteTree := utils.BuildSiteTree(allPosts)

			b.renderService.RenderIndex(destPath, models.PageData{Title: cfg.Title, Posts: pagePosts, PinnedPosts: curPinned, BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title, Description: cfg.Description, Permalink: permalink, Image: cfg.BaseURL + "/static/images/cards/home.webp", Paginator: paginator, SiteTree: siteTree, Config: cfg})
		}(i)
	}
	wg.Wait()
}

func (b *Builder) renderTags(tagMap map[string][]models.PostMetadata, forceSocialRebuild bool) {
	var allTags []models.TagData
	for t, posts := range tagMap {
		allTags = append(allTags, models.TagData{Name: t, Count: len(posts), Link: fmt.Sprintf("%s/tags/%s.html", b.cfg.BaseURL, t)})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

	// Generate Tags Index Card
	tagsIndexCard := filepath.Join(b.cfg.OutputDir, "static/images/cards/tags/index.webp")

	indexContent := fmt.Sprintf("All Topics|%d", len(tagMap))
	indexHash := hashString(indexContent)
	needsIndexGen := false

	if _, err := os.Stat(tagsIndexCard); os.IsNotExist(err) || forceSocialRebuild {
		needsIndexGen = true
	} else if b.cacheService != nil {
		cachedHash, _ := b.cacheService.GetSocialCardHash("tags/index")
		if cachedHash != indexHash {
			needsIndexGen = true
		}
	}

	if needsIndexGen {
		_ = os.MkdirAll(filepath.Dir(tagsIndexCard), 0755)
		faviconPath := ""
		if b.cfg.Logo != "" {
			faviconPath = b.cfg.Logo
		} else {
			faviconPath = filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
		}
		err := generators.GenerateSocialCardToDisk(b.SourceFs, &b.cfg.SocialCards, b.cfg.Title, "All Topics", fmt.Sprintf("Browse all %d topics", len(tagMap)), "Topics", tagsIndexCard, faviconPath)
		if err == nil && b.cacheService != nil {
			_ = b.cacheService.SetSocialCardHash("tags/index", indexHash)
		}
	}

	// Generate Tags Index
	// Force Weight: 0 so layout doesn't crash
	b.renderService.RenderPage(filepath.Join(b.cfg.OutputDir, "tags/index.html"), models.PageData{
		Title: "All Tags", IsTagsIndex: true, AllTags: allTags,
		BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
		Permalink: b.cfg.BaseURL + "/tags/index.html",
		Image:     b.cfg.BaseURL + "/static/images/cards/tags/index.webp",
		TabTitle:  "All Topics | " + b.cfg.Title, Config: b.cfg,
		Weight: 0, // Fix for docs theme layout
	})

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for t, posts := range tagMap {
		wg.Add(1)
		sem <- struct{}{}
		go func(t string, posts []models.PostMetadata) {
			defer wg.Done()
			defer func() { <-sem }()

			// Generate Tag Card
			tagCard := filepath.Join(b.cfg.OutputDir, fmt.Sprintf("static/images/cards/tags/%s.webp", strings.ToLower(t)))

			// Hash: Tag Name + Post Count
			// This ensures update when count changes
			tagContent := fmt.Sprintf("%s|%d", t, len(posts))
			tagHash := hashString(tagContent)
			needsTagGen := false

			if _, err := os.Stat(tagCard); os.IsNotExist(err) || forceSocialRebuild {
				needsTagGen = true
			} else if b.cacheService != nil {
				cachedHash, _ := b.cacheService.GetSocialCardHash("tags/" + strings.ToLower(t))
				if cachedHash != tagHash {
					needsTagGen = true
				}
			}

			if needsTagGen {
				_ = os.MkdirAll(filepath.Dir(tagCard), 0755)
				faviconPath := ""
				if b.cfg.Logo != "" {
					faviconPath = b.cfg.Logo
				} else {
					faviconPath = filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
				}
				err := generators.GenerateSocialCardToDisk(b.SourceFs, &b.cfg.SocialCards, b.cfg.Title, "#"+t, fmt.Sprintf("%d posts about %s", len(posts), t), "Topic", tagCard, faviconPath)
				if err == nil && b.cacheService != nil {
					_ = b.cacheService.SetSocialCardHash("tags/"+strings.ToLower(t), tagHash)
				}
			}

			utils.SortPosts(posts)
			b.renderService.RenderPage(filepath.Join(b.cfg.OutputDir, fmt.Sprintf("tags/%s.html", t)), models.PageData{
				Title: "#" + t, IsIndex: true, Posts: posts,
				BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
				Permalink: fmt.Sprintf("%s/tags/%s.html", b.cfg.BaseURL, t),
				Image:     fmt.Sprintf("%s/static/images/cards/tags/%s.webp", b.cfg.BaseURL, strings.ToLower(t)),
				TabTitle:  "#" + t + " | " + b.cfg.Title, Config: b.cfg,
				Weight: 0, // Fix for docs theme layout
			})
		}(t, posts)
	}
	wg.Wait()
}
