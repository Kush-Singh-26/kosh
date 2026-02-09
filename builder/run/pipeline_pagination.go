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
	homeCardPath := "public/static/images/cards/home.webp"

	// Check content hash
	cardContent := cfg.Title + "|" + cfg.Description
	currentHash := hashString(cardContent)
	needsGen := false

	if _, err := os.Stat(homeCardPath); os.IsNotExist(err) || force {
		needsGen = true
	} else if b.cacheManager != nil {
		cachedHash, _ := b.cacheManager.GetSocialCardHash("home")
		if cachedHash != currentHash {
			needsGen = true
		}
	}

	if needsGen {
		_ = b.DestFs.MkdirAll(filepath.Dir(homeCardPath), 0755)
		faviconPath := filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
		_ = os.MkdirAll(filepath.Dir(homeCardPath), 0755)

		desc := cfg.Description
		if len(desc) > 100 {
			desc = desc[:97] + "..."
		}

		err := generators.GenerateSocialCardToDisk(b.SourceFs, cfg.Title, desc, "Latest Posts", homeCardPath, faviconPath)
		if err != nil {
			fmt.Printf("⚠️ Failed to generate home card: %v\n", err)
		} else if b.cacheManager != nil {
			_ = b.cacheManager.SetSocialCardHash("home", currentHash)
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
			destPath, permalink := "public/index.html", cfg.BaseURL+"/"
			if i > 1 {
				destPath = fmt.Sprintf("public/page/%d/index.html", i)
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
			b.rnd.RenderIndex(destPath, models.PageData{Title: cfg.Title, Posts: pagePosts, PinnedPosts: curPinned, BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title, Description: cfg.Description, Permalink: permalink, Image: cfg.BaseURL + "/static/images/cards/home.webp", Paginator: paginator, Config: cfg})
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
	tagsIndexCard := "public/static/images/cards/tags/index.webp"

	indexContent := fmt.Sprintf("All Topics|%d", len(tagMap))
	indexHash := hashString(indexContent)
	needsIndexGen := false

	if _, err := os.Stat(tagsIndexCard); os.IsNotExist(err) || forceSocialRebuild {
		needsIndexGen = true
	} else if b.cacheManager != nil {
		cachedHash, _ := b.cacheManager.GetSocialCardHash("tags/index")
		if cachedHash != indexHash {
			needsIndexGen = true
		}
	}

	if needsIndexGen {
		_ = os.MkdirAll(filepath.Dir(tagsIndexCard), 0755)
		faviconPath := filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
		err := generators.GenerateSocialCardToDisk(b.SourceFs, "All Topics", fmt.Sprintf("Browse all %d topics", len(tagMap)), "Topics", tagsIndexCard, faviconPath)
		if err == nil && b.cacheManager != nil {
			_ = b.cacheManager.SetSocialCardHash("tags/index", indexHash)
		}
	}

	b.rnd.RenderPage("public/tags/index.html", models.PageData{Title: "All Tags", IsTagsIndex: true, AllTags: allTags, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion, Permalink: b.cfg.BaseURL + "/tags/index.html", Image: b.cfg.BaseURL + "/static/images/cards/tags/index.webp", TabTitle: "All Topics | " + b.cfg.Title, Config: b.cfg})

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for t, posts := range tagMap {
		wg.Add(1)
		sem <- struct{}{}
		go func(t string, posts []models.PostMetadata) {
			defer wg.Done()
			defer func() { <-sem }()

			// Generate Tag Card
			tagCard := fmt.Sprintf("public/static/images/cards/tags/%s.webp", strings.ToLower(t))

			// Hash: Tag Name + Post Count
			// This ensures update when count changes
			tagContent := fmt.Sprintf("%s|%d", t, len(posts))
			tagHash := hashString(tagContent)
			needsTagGen := false

			if _, err := os.Stat(tagCard); os.IsNotExist(err) || forceSocialRebuild {
				needsTagGen = true
			} else if b.cacheManager != nil {
				cachedHash, _ := b.cacheManager.GetSocialCardHash("tags/" + strings.ToLower(t))
				if cachedHash != tagHash {
					needsTagGen = true
				}
			}

			if needsTagGen {
				_ = os.MkdirAll(filepath.Dir(tagCard), 0755)
				faviconPath := filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
				err := generators.GenerateSocialCardToDisk(b.SourceFs, "#"+t, fmt.Sprintf("%d posts about %s", len(posts), t), "Topic", tagCard, faviconPath)
				if err == nil && b.cacheManager != nil {
					_ = b.cacheManager.SetSocialCardHash("tags/"+strings.ToLower(t), tagHash)
				}
			}

			utils.SortPosts(posts)
			b.rnd.RenderPage(fmt.Sprintf("public/tags/%s.html", t), models.PageData{Title: "#" + t, IsIndex: true, Posts: posts, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion, Permalink: fmt.Sprintf("%s/tags/%s.html", b.cfg.BaseURL, t), Image: fmt.Sprintf("%s/static/images/cards/tags/%s.webp", b.cfg.BaseURL, strings.ToLower(t)), TabTitle: "#" + t + " | " + b.cfg.Title, Config: b.cfg})
		}(t, posts)
	}
	wg.Wait()
}
