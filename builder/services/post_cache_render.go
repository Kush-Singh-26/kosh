package services

import (
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/afero"

	"my-ssg/builder/cache"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

func (s *postServiceImpl) RenderCachedPosts() {
	if s.cache == nil {
		return
	}

	var ids []string
	var err error
	if lister, ok := s.cache.(interface{ ListAllPosts() ([]string, error) }); ok {
		ids, err = lister.ListAllPosts()
	} else {
		return
	}

	if err != nil {
		s.logger.Warn("Failed to list posts from cache", "error", err)
		return
	}

	type CachedPostData struct {
		Meta *cache.PostMeta
		HTML []byte
	}

	cachedData := make(map[string]*CachedPostData, len(ids))
	postsByVersion := make(map[string][]models.PostMetadata)

	cachedPostsMap, err := s.cache.GetPostsByIDs(ids)
	if err != nil {
		s.logger.Warn("Failed to batch read from cache", "error", err)
		return
	}

	for id, meta := range cachedPostsMap {
		htmlBytes, _ := s.cache.GetHTMLContent(meta)
		if htmlBytes == nil {
			continue
		}
		cachedData[id] = &CachedPostData{Meta: meta, HTML: htmlBytes}

		post := models.PostMetadata{
			Title: meta.Title, Link: meta.Link, Weight: meta.Weight, Version: meta.Version,
			DateObj: meta.Date,
		}
		postsByVersion[meta.Version] = append(postsByVersion[meta.Version], post)
	}

	siteTrees := make(map[string][]*models.TreeNode)
	for ver, posts := range postsByVersion {
		utils.SortPosts(posts)
		siteTrees[ver] = utils.BuildSiteTree(posts)
	}

	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for id, data := range cachedData {
		wg.Add(1)
		sem <- struct{}{}
		go func(postID string, cp *CachedPostData) {
			defer wg.Done()
			defer func() { <-sem }()

			relPath := cp.Meta.Path
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

			cleanHtmlRelPath := htmlRelPath
			if cp.Meta.Version != "" {
				cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(cp.Meta.Version)+"/")
			}

			var destPath string
			if cp.Meta.Version != "" {
				destPath = filepath.Join("public", cp.Meta.Version, cleanHtmlRelPath)
			} else {
				destPath = filepath.Join("public", htmlRelPath)
			}

			if s.cfg.Features.RawMarkdown {
				mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
				if _, err := os.Stat(mdDestPath); os.IsNotExist(err) {
					sourcePath := filepath.Join(s.cfg.ContentDir, relPath)
					sourceBytes, _ := afero.ReadFile(s.sourceFs, sourcePath)
					if len(sourceBytes) > 0 {
						_ = s.destFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
						_ = afero.WriteFile(s.destFs, mdDestPath, sourceBytes, 0644)
					}
				}
			}

			imagePath := s.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
			if img, ok := cp.Meta.Meta["image"].(string); ok {
				if s.cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = s.cfg.BaseURL + img
			}

			var toc []models.TOCEntry
			for _, t := range cp.Meta.TOC {
				toc = append(toc, models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level})
			}

			versionPosts := postsByVersion[cp.Meta.Version]
			currentPost := models.PostMetadata{
				Title: cp.Meta.Title, Link: cp.Meta.Link, Weight: cp.Meta.Weight, Version: cp.Meta.Version,
				DateObj: cp.Meta.Date,
			}
			prev, next := utils.FindPrevNext(currentPost, versionPosts)

			s.renderer.RenderPage(destPath, models.PageData{
				Title: cp.Meta.Title, Description: cp.Meta.Description, Content: template.HTML(string(cp.HTML)),
				Meta: cp.Meta.Meta, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
				TabTitle: cp.Meta.Title + " | " + s.cfg.Title, Permalink: cp.Meta.Link, Image: imagePath,
				TOC: toc, Config: s.cfg,
				SiteTree:       siteTrees[cp.Meta.Version],
				CurrentVersion: cp.Meta.Version,
				IsOutdated:     cp.Meta.Version != "",
				Versions:       s.cfg.GetVersionsMetadata(cp.Meta.Version),
				PrevPage:       prev,
				NextPage:       next,
			})

			s.metrics.IncrementPostsProcessed()
			s.metrics.IncrementCacheHit()
		}(id, data)
	}
	wg.Wait()
}
