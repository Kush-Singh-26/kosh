package run

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	bolt "go.etcd.io/bbolt"

	"my-ssg/builder/cache"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

// Constants for magic numbers
const (
	wordsPerMinute     = 120.0     // Average reading speed for calculating reading time
	smallFileThreshold = 64 * 1024 // 64KB threshold for small files in VFS sync
)

// socialCardTask represents a social card generation task
type socialCardTask struct {
	path, relPath, cardDestPath string
	metaData                    map[string]interface{}
	frontmatterHash             string
}

func (b *Builder) processPosts(shouldForce, forceSocialRebuild, outputMissing bool) ([]models.PostMetadata, []models.PostMetadata, map[string][]models.PostMetadata, []models.IndexedPost, bool, bool) {
	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		indexedPosts   []models.IndexedPost
		tagMap         = make(map[string][]models.PostMetadata)
		postsByVersion = make(map[string][]models.PostMetadata)
		has404         bool
		anyPostChanged bool
		processedCount int32
		mu             sync.Mutex
		wg             sync.WaitGroup
	)

	// Batch storage for BoltDB commit
	var (
		batchMu          sync.Mutex
		newPostsMeta     []*cache.PostMeta
		newSearchRecords = make(map[string]*cache.SearchRecord)
		newDeps          = make(map[string]*cache.Dependencies)
	)

	type RenderContext struct {
		DestPath string
		Data     models.PageData
		Version  string
	}

	var files []string
	var fileVersions []string
	_ = afero.Walk(b.SourceFs, "content", func(path string, info fs.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".md") && !strings.Contains(path, "_index.md") {
			if strings.Contains(path, "404.md") {
				has404 = true
			} else {
				ver, _ := utils.GetVersionFromPath(path)
				files = append(files, path)
				fileVersions = append(fileVersions, ver)
			}
		}
		return nil
	})

	renderQueue := make([]RenderContext, len(files))

	numWorkers := runtime.NumCPU()
	if numWorkers > 12 {
		numWorkers = 12
	}
	sem := make(chan struct{}, numWorkers)

	cardQueue := make(chan socialCardTask, numWorkers*4)
	var cardWg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		cardWg.Add(1)
		go func() {
			defer cardWg.Done()
			for task := range cardQueue {
				b.generateSocialCard(task)
			}
		}()
	}

	// Phase 0: Load global metadata from cache for complete sidebar/neighbor context
	allMetadataMap := make(map[string]models.PostMetadata)
	if b.cacheManager != nil {
		ids, _ := b.cacheManager.ListAllPosts()
		cachedPosts, _ := b.cacheManager.GetPostsByIDs(ids)
		for _, cp := range cachedPosts {
			allMetadataMap[cp.Link] = models.PostMetadata{
				Title: cp.Title, Link: cp.Link, Weight: cp.Weight, Version: cp.Version,
				DateObj: cp.Date, ReadingTime: cp.ReadingTime, Description: cp.Description,
				Tags: cp.Tags, Pinned: cp.Pinned, Draft: cp.Draft,
			}
		}
	}

	// Mutex for protecting allMetadataMap during concurrent access
	var metadataMu sync.RWMutex

	for i, path := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, path string, version string) {
			defer wg.Done()
			defer func() { <-sem }()

			relPath, _ := utils.SafeRel("content", path)
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

			cleanHtmlRelPath := htmlRelPath
			if version != "" {
				cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
			}

			var destPath string
			if version != "" {
				destPath = filepath.Join("public", version, cleanHtmlRelPath)
			} else {
				destPath = filepath.Join("public", htmlRelPath)
			}

			// 1. Resolve from Cache
			var cachedMeta *cache.PostMeta
			var cachedSearch *cache.SearchRecord
			var cachedHTML []byte
			var err error
			var info os.FileInfo
			exists := false

			if b.cacheManager != nil {
				cachedMeta, err = b.cacheManager.GetPostByPath(relPath)
				if err == nil && cachedMeta != nil {
					exists = true
					info, _ = b.SourceFs.Stat(path)
					if info != nil && info.ModTime().Unix() > cachedMeta.ModTime {
						exists = false
					}
				}
			}

			useCache := exists && !shouldForce

			var cachedHash string
			if b.cacheManager != nil && !useCache {
				cachedHash, _ = b.cacheManager.GetSocialCardHash(relPath)
			} else if useCache && cachedMeta != nil {
				cachedHash = cachedMeta.ContentHash
			}

			var htmlContent string
			var metaData map[string]interface{}
			var post models.PostMetadata
			var searchRecord models.PostRecord
			var wordFreqs map[string]int
			var docLen int
			var words []string
			var toc []models.TOCEntry
			var frontmatterHash string
			var plainText string

			if useCache {
				cachedHTML, err = b.cacheManager.GetHTMLContent(cachedMeta)
				if err != nil || cachedHTML == nil {
					useCache = false
				} else {
					cachedSearch, err = b.cacheManager.GetSearchRecord(cachedMeta.PostID)
					if err != nil || cachedSearch == nil {
						useCache = false
					}
				}
			}

			if useCache {
				b.metrics.IncrementCacheHit()
				htmlContent = string(cachedHTML)
				metaData = cachedMeta.Meta
				frontmatterHash = cachedMeta.ContentHash

				metadataMu.RLock()
				post = allMetadataMap[cachedMeta.Link]
				metadataMu.RUnlock()

				for _, t := range cachedMeta.TOC {
					toc = append(toc, models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level})
				}

				searchRecord = models.PostRecord{
					Title: cachedSearch.Title, Link: htmlRelPath, Description: cachedMeta.Description,
					Tags: cachedMeta.Tags, Content: cachedSearch.Content, Version: cachedMeta.Version,
				}
				docLen = cachedSearch.DocLen
				wordFreqs = cachedSearch.BM25Data
			} else {
				b.metrics.IncrementCacheMiss()
				if info == nil {
					info, _ = b.SourceFs.Stat(path)
				}
				source, _ := afero.ReadFile(b.SourceFs, path)

				// Copy raw markdown to output for "View Source" feature
				if b.cfg.Features.RawMarkdown {
					// Use filepath to handle OS-specific path separators correctly
					mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
					_ = b.DestFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
					_ = afero.WriteFile(b.DestFs, mdDestPath, source, 0644)
				}

				ctx := parser.NewContext()
				ctx.Set(mdParser.ContextKeyFilePath, path)
				docNode := b.md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))
				var buf bytes.Buffer

				_ = b.md.Renderer().Render(&buf, source, docNode)
				htmlContent = buf.String()

				if pairs := mdParser.GetD2SVGPairSlice(ctx); pairs != nil {
					htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
				}

				var diagramCache map[string]string
				if b.diagramAdapter != nil {
					diagramCache = b.diagramAdapter.AsMap()
				}

				if strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(") {
					htmlContent = mdParser.RenderMathForHTML(htmlContent, b.native, diagramCache, &b.mu)
				}
				if b.cfg.CompressImages {
					htmlContent = utils.ReplaceToWebP(htmlContent)
				}

				metaData = meta.Get(ctx)
				dateStr := utils.GetString(metaData, "date")
				dateObj, _ := time.Parse("2006-01-02", dateStr)
				isPinned, _ := metaData["pinned"].(bool)
				weight, _ := metaData["weight"].(int)
				if w, ok := metaData["weight"].(float64); ok && weight == 0 {
					weight = int(w)
				}
				wordCount := len(strings.Fields(string(source)))
				toc = mdParser.GetTOC(ctx)

				postLink := utils.BuildPostLink(b.cfg.BaseURL, version, cleanHtmlRelPath)

				post = models.PostMetadata{
					Title: utils.GetString(metaData, "title"), Link: postLink,
					Description: utils.GetString(metaData, "description"), Tags: utils.GetSlice(metaData, "tags"),
					ReadingTime: int(math.Ceil(float64(wordCount) / wordsPerMinute)), Pinned: isPinned, Weight: weight,
					DateObj: dateObj, Draft: utils.GetBool(metaData, "draft"), Version: version,
				}

				plainText = mdParser.ExtractPlainText(docNode, source)
				searchRecord = models.PostRecord{
					Title: post.Title, Link: htmlRelPath, Description: post.Description,
					Tags: post.Tags, Content: plainText, Version: version,
				}
				words = search.Tokenize(strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content))
				docLen = len(words)
				wordFreqs = make(map[string]int)
				for _, w := range words {
					if len(w) >= 2 {
						wordFreqs[w]++
					}
				}
				frontmatterHash, _ = utils.GetFrontmatterHash(metaData)
			}

			if post.Draft && !b.cfg.IncludeDrafts {
				return
			}

			cardDestPath := filepath.Join("public", "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp")
			_ = b.DestFs.MkdirAll(filepath.Dir(cardDestPath), 0755)

			cardExists := false
			if info, err := os.Stat(cardDestPath); err == nil && !info.IsDir() {
				if sourceInfo, err := b.SourceFs.Stat(path); err == nil {
					if info.ModTime().After(sourceInfo.ModTime()) {
						cardExists = true
					}
				}
			}

			if forceSocialRebuild || (cachedHash != frontmatterHash && !cardExists) {
				cardQueue <- socialCardTask{
					path: relPath, relPath: strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
					cardDestPath: cardDestPath, metaData: metaData, frontmatterHash: frontmatterHash,
				}
			} else if cardExists {
				if b.cacheManager != nil && cachedHash == "" {
					_ = b.cacheManager.SetSocialCardHash(relPath, frontmatterHash)
				}
			}

			imagePath := b.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
			if img, ok := metaData["image"].(string); ok {
				if b.cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = b.cfg.BaseURL + img
			}

			willRender := false
			if outputMissing {
				willRender = true
			} else if useCache {
				if _, err := os.Stat(destPath); os.IsNotExist(err) {
					willRender = true
				}
			} else {
				if info == nil {
					info, _ = b.SourceFs.Stat(path)
				}
				if destInfo, err := os.Stat(destPath); err != nil || !destInfo.ModTime().After(info.ModTime()) {
					willRender = true
				}
			}

			// Copy raw markdown to output for "View Source" feature (for cached posts too)
			if b.cfg.Features.RawMarkdown {
				mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
				if _, err := os.Stat(mdDestPath); os.IsNotExist(err) {
					sourceBytes, _ := afero.ReadFile(b.SourceFs, path)
					if len(sourceBytes) > 0 {
						_ = b.DestFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
						_ = afero.WriteFile(b.DestFs, mdDestPath, sourceBytes, 0644)
					}
				}
			}

			if willRender {
				renderQueue[idx] = RenderContext{
					DestPath: destPath,
					Version:  version,
					Data: models.PageData{
						Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
						Meta: metaData, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
						TabTitle: post.Title + " | " + b.cfg.Title, Permalink: post.Link, Image: imagePath,
						TOC: toc, Config: b.cfg,
						CurrentVersion: version,
						IsOutdated:     version != "",
						Versions:       b.cfg.GetVersionsMetadata(version),
					},
				}
				mu.Lock()
				anyPostChanged = true
				mu.Unlock()
			}

			metadataMu.Lock()
			allMetadataMap[post.Link] = post
			metadataMu.Unlock()

			mu.Lock()
			searchRecord.ID = len(indexedPosts)
			indexedPosts = append(indexedPosts, models.IndexedPost{Record: searchRecord, WordFreqs: wordFreqs, DocLen: docLen})
			mu.Unlock()

			if !useCache && b.cacheManager != nil {
				postID := cache.GeneratePostID("", relPath)
				newMeta := &cache.PostMeta{
					PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
					ContentHash: frontmatterHash, Title: post.Title, Date: post.DateObj,
					Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
					Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
					Meta: metaData, TOC: toc, Version: version,
				}
				_ = b.cacheManager.StoreHTMLForPost(newMeta, []byte(htmlContent))
				newSearch := &cache.SearchRecord{
					Title: post.Title, BM25Data: wordFreqs, DocLen: docLen, Content: plainText,
				}
				newDep := &cache.Dependencies{Tags: post.Tags}

				batchMu.Lock()
				newPostsMeta = append(newPostsMeta, newMeta)
				newSearchRecords[postID] = newSearch
				newDeps[postID] = newDep
				batchMu.Unlock()
			}

			b.metrics.IncrementPostsProcessed()
			_ = atomic.AddInt32(&processedCount, 1)
		}(i, path, fileVersions[i])
	}
	wg.Wait()

	// Final Metadata Grouping (merges Cache + Source)
	for _, p := range allMetadataMap {
		postsByVersion[p.Version] = append(postsByVersion[p.Version], p)
		if p.Version == "" {
			for _, t := range p.Tags {
				tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], p)
			}
			if p.Pinned {
				pinnedPosts = append(pinnedPosts, p)
			} else {
				allPosts = append(allPosts, p)
			}
		}
	}

	siteTrees := make(map[string][]*models.TreeNode)
	for ver, posts := range postsByVersion {
		utils.SortPosts(posts)
		siteTrees[ver] = utils.BuildSiteTree(posts)
	}

	renderWg := sync.WaitGroup{}
	for i := range renderQueue {
		task := &renderQueue[i]
		if task.DestPath == "" {
			continue
		}

		// Inject neighbors (Prev/Next)
		versionPosts := postsByVersion[task.Version]
		currentPost := models.PostMetadata{
			Title: task.Data.Title, Link: task.Data.Permalink, Weight: task.Data.Weight, Version: task.Version,
		}

		// Ensure we match the actual metadata object to get DateObj for sorting if needed
		for _, p := range versionPosts {
			if p.Link == task.Data.Permalink {
				currentPost = p
				break
			}
		}

		prev, next := utils.FindPrevNext(currentPost, versionPosts)
		task.Data.PrevPage = prev
		task.Data.NextPage = next

		renderWg.Add(1)
		sem <- struct{}{}
		go func(t RenderContext) {
			defer renderWg.Done()
			defer func() { <-sem }()
			t.Data.SiteTree = siteTrees[t.Version]
			b.rnd.RenderPage(t.DestPath, t.Data)
		}(*task)
	}
	renderWg.Wait()

	if b.cacheManager != nil && len(newPostsMeta) > 0 {
		if err := b.cacheManager.BatchCommit(newPostsMeta, newSearchRecords, newDeps); err != nil {
			b.logger.Warn("Failed to commit cache batch", "error", err)
		}
	}

	close(cardQueue)
	cardWg.Wait()
	runtime.GC()

	// Sort posts to ensure consistent ordering
	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	return allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404
}

func (b *Builder) renderCachedPosts() {
	if b.cacheManager == nil {
		fmt.Println("⚠️ Cache manager not available, skipping fast render.")
		return
	}

	ids, err := b.cacheManager.ListAllPosts()
	if err != nil {
		b.logger.Warn("Failed to list posts from cache", "error", err)
		return
	}

	type CachedPostData struct {
		Meta *cache.PostMeta
		HTML []byte
	}

	cachedData := make(map[string]*CachedPostData, len(ids))
	postsByVersion := make(map[string][]models.PostMetadata)

	err = b.cacheManager.DB().View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(cache.BucketPosts))
		for _, id := range ids {
			data := postsBucket.Get([]byte(id))
			if data == nil {
				continue
			}
			var meta cache.PostMeta
			if err := cache.Decode(data, &meta); err != nil {
				continue
			}
			htmlBytes, _ := b.cacheManager.GetHTMLContent(&meta)
			if htmlBytes == nil {
				continue
			}
			cachedData[id] = &CachedPostData{Meta: &meta, HTML: htmlBytes}

			post := models.PostMetadata{
				Title: meta.Title, Link: meta.Link, Weight: meta.Weight, Version: meta.Version,
				DateObj: meta.Date,
			}
			postsByVersion[meta.Version] = append(postsByVersion[meta.Version], post)
		}
		return nil
	})

	if err != nil {
		b.logger.Warn("Failed to batch read from cache", "error", err)
		return
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

			// Copy raw markdown to output for "View Source" feature
			if b.cfg.Features.RawMarkdown {
				mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
				if _, err := os.Stat(mdDestPath); os.IsNotExist(err) {
					sourcePath := filepath.Join("content", relPath)
					sourceBytes, _ := afero.ReadFile(b.SourceFs, sourcePath)
					if len(sourceBytes) > 0 {
						_ = b.DestFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
						_ = afero.WriteFile(b.DestFs, mdDestPath, sourceBytes, 0644)
					}
				}
			}

			imagePath := b.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
			if img, ok := cp.Meta.Meta["image"].(string); ok {
				if b.cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = b.cfg.BaseURL + img
			}

			var toc []models.TOCEntry
			for _, t := range cp.Meta.TOC {
				toc = append(toc, models.TOCEntry{ID: t.ID, Text: t.Text, Level: t.Level})
			}

			// FIND NEIGHBORS (Prev/Next) within cached version context
			versionPosts := postsByVersion[cp.Meta.Version]
			currentPost := models.PostMetadata{
				Title: cp.Meta.Title, Link: cp.Meta.Link, Weight: cp.Meta.Weight, Version: cp.Meta.Version,
				DateObj: cp.Meta.Date,
			}
			prev, next := utils.FindPrevNext(currentPost, versionPosts)

			b.rnd.RenderPage(destPath, models.PageData{
				Title: cp.Meta.Title, Description: cp.Meta.Description, Content: template.HTML(string(cp.HTML)),
				Meta: cp.Meta.Meta, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
				TabTitle: cp.Meta.Title + " | " + b.cfg.Title, Permalink: cp.Meta.Link, Image: imagePath,
				TOC: toc, Config: b.cfg,
				SiteTree:       siteTrees[cp.Meta.Version],
				CurrentVersion: cp.Meta.Version,
				IsOutdated:     cp.Meta.Version != "",
				Versions:       b.cfg.GetVersionsMetadata(cp.Meta.Version),
				PrevPage:       prev,
				NextPage:       next,
			})

			b.metrics.IncrementPostsProcessed()
			b.metrics.IncrementCacheHit()
		}(id, data)
	}
	wg.Wait()
}

func (b *Builder) generateSocialCard(t socialCardTask) {
	cachedCardPath := filepath.Join(".kosh-cache", "social-cards", t.frontmatterHash+".webp")
	cachedFile, err := os.Open(cachedCardPath)
	if err == nil && t.frontmatterHash != "" {
		defer cachedFile.Close()
		out, err := b.DestFs.Create(t.cardDestPath)
		if err == nil {
			defer out.Close()
			if _, err := io.Copy(out, cachedFile); err == nil {
				if b.cacheManager != nil {
					_ = b.cacheManager.SetSocialCardHash(t.path, t.frontmatterHash)
				}
				return
			}
		}
	}

	logoPath := ""
	if b.cfg.Logo != "" {
		logoPath = b.cfg.Logo
	} else {
		logoPath = filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
	}
	err = generators.GenerateSocialCardToDisk(b.SourceFs, b.cfg.Title, utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"), utils.GetString(t.metaData, "date"), cachedCardPath, logoPath)

	if err == nil {
		if data, err := os.ReadFile(cachedCardPath); err == nil {
			_ = b.DestFs.MkdirAll(filepath.Dir(t.cardDestPath), 0755)
			_ = afero.WriteFile(b.DestFs, t.cardDestPath, data, 0644)
		}
		if b.cacheManager != nil && t.frontmatterHash != "" {
			_ = b.cacheManager.SetSocialCardHash(t.path, t.frontmatterHash)
		}
	} else {
		_ = generators.GenerateSocialCard(b.DestFs, b.SourceFs, b.cfg.Title, utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"), utils.GetString(t.metaData, "date"), t.cardDestPath, logoPath)
	}
}
