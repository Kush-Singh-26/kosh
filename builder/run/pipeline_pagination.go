package run

import (
	"fmt"
	"math"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func (b *Builder) renderPagination(allPosts, pinnedPosts []models.PostMetadata) {
	cfg := b.cfg
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
				b.DestFs.MkdirAll(filepath.Dir(destPath), 0755)
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

	b.rnd.RenderPage("public/tags/index.html", models.PageData{Title: "All Tags", IsTagsIndex: true, AllTags: allTags, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion, Permalink: b.cfg.BaseURL + "/tags/index.html", Image: b.cfg.BaseURL + "/static/images/cards/tags/index.webp", TabTitle: "All Topics | " + b.cfg.Title, Config: b.cfg})

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for t, posts := range tagMap {
		wg.Add(1)
		sem <- struct{}{}
		go func(t string, posts []models.PostMetadata) {
			defer wg.Done()
			defer func() { <-sem }()
			utils.SortPosts(posts)
			b.rnd.RenderPage(fmt.Sprintf("public/tags/%s.html", t), models.PageData{Title: "#" + t, IsIndex: true, Posts: posts, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion, Permalink: fmt.Sprintf("%s/tags/%s.html", b.cfg.BaseURL, t), Image: fmt.Sprintf("%s/static/images/cards/tags/%s.webp", b.cfg.BaseURL, strings.ToLower(t)), TabTitle: "#" + t + " | " + b.cfg.Title, Config: b.cfg})
		}(t, posts)
	}
	wg.Wait()
}
