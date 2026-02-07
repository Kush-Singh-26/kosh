package run

import (
	"bytes"
	"fmt"
	"html/template"
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

	"my-ssg/builder/cache"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

func (b *Builder) processPosts(shouldForce, forceSocialRebuild bool) ([]models.PostMetadata, []models.PostMetadata, map[string][]models.PostMetadata, []models.IndexedPost, bool, bool) {
	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		indexedPosts   []models.IndexedPost
		tagMap         = make(map[string][]models.PostMetadata)
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

	type socialCardTask struct {
		path, relPath, cardDestPath string
		metaData                    map[string]interface{}
		frontmatterHash             string
	}
	var socialCardTasks []socialCardTask
	var socialTasksMu sync.Mutex

	var files []string
	_ = afero.Walk(b.SourceFs, "content", func(path string, info fs.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".md") && !strings.Contains(path, "_index.md") {
			if strings.Contains(path, "404.md") {
				has404 = true
			} else {
				files = append(files, path)
			}
		}
		return nil
	})

	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	fmt.Printf("📝 Processing %d posts...\n", len(files))

	for _, path := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			info, _ := b.SourceFs.Stat(path)
			relPath, _ := filepath.Rel("content", path)
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
			destPath := filepath.Join("public", htmlRelPath)

			// 1. Try to load from BoltDB Cache
			var cachedMeta *cache.PostMeta
			var cachedSearch *cache.SearchRecord
			var cachedHTML []byte
			var err error
			exists := false

			if b.cacheManager != nil {
				cachedMeta, err = b.cacheManager.GetPostByPath(relPath)
				if err == nil && cachedMeta != nil {
					exists = true
					// Check freshness
					if info.ModTime().Unix() > cachedMeta.ModTime {
						exists = false // Stale
					}
					// Check content hash or other validity if needed in future
				}
			}

			// Get cached social card hash from BoltDB
			var cachedHash string
			if b.cacheManager != nil {
				cachedHash, _ = b.cacheManager.GetSocialCardHash(relPath)
			}

			useCache := exists && !shouldForce

			var htmlContent string
			var metaData map[string]interface{}
			var post models.PostMetadata
			var searchRecord models.PostRecord
			var wordFreqs map[string]int
			var docLen int
			var toc []models.TOCEntry
			var frontmatterHash string
			var dependencies cache.Dependencies
			var plainText string

			// Load content from cache if valid
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
				htmlContent = string(cachedHTML)
				metaData = cachedMeta.Meta
				frontmatterHash = cachedMeta.ContentHash // Using ContentHash as FrontmatterHash per migration plan

				// Reconstruct PostMetadata
				post = models.PostMetadata{
					Title:       cachedMeta.Title,
					Link:        cachedMeta.Link,
					Description: cachedMeta.Description,
					Tags:        cachedMeta.Tags,
					ReadingTime: cachedMeta.ReadingTime,
					Pinned:      cachedMeta.Pinned,
					Draft:       cachedMeta.Draft,
					DateObj:     cachedMeta.Date,
					HasMath:     cachedMeta.HasMath,
					HasMermaid:  cachedMeta.HasMermaid,
				}

				// Convert TOC
				for _, t := range cachedMeta.TOC {
					toc = append(toc, models.TOCEntry{
						ID:    t.ID,
						Text:  t.Text,
						Level: t.Level,
					})
				}

				// Reconstruct Search/Index Data
				searchRecord = models.PostRecord{
					Title: cachedSearch.Title, Link: cachedMeta.Link, Description: cachedMeta.Description, Tags: cachedMeta.Tags,
					Content: cachedSearch.Content,
				}

				docLen = cachedSearch.DocLen
				wordFreqs = cachedSearch.BM25Data
			} else {
				// Parse and Render
				source, _ := afero.ReadFile(b.SourceFs, path)
				ctx := parser.NewContext()
				docNode := b.md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))
				var buf bytes.Buffer

				_ = b.md.Renderer().Render(&buf, source, docNode)
				htmlContent = buf.String()

				if pairs := mdParser.GetD2SVGPairSlice(ctx); pairs != nil {
					htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
				}

				// Use DiagramAdapter map equivalent
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
				wordCount := len(strings.Fields(string(source)))

				toc = mdParser.GetTOC(ctx)

				post = models.PostMetadata{
					Title: utils.GetString(metaData, "title"), Link: b.cfg.BaseURL + "/" + htmlRelPath,
					Description: utils.GetString(metaData, "description"), Tags: utils.GetSlice(metaData, "tags"),
					ReadingTime: int(math.Ceil(float64(wordCount) / 120.0)), Pinned: isPinned,
					DateObj: dateObj, HasMath: strings.Contains(string(source), "$"), HasMermaid: mdParser.HasD2(ctx),
					Draft: utils.GetBool(metaData, "draft"),
				}

				plainText = mdParser.ExtractPlainText(docNode, source)
				searchRecord = models.PostRecord{Title: post.Title, Link: htmlRelPath, Description: post.Description, Tags: post.Tags, Content: plainText}
				words := search.Tokenize(strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content))
				docLen = len(words)
				wordFreqs = make(map[string]int)
				for _, w := range words {
					if len(w) >= 2 {
						wordFreqs[w]++
					}
				}
				frontmatterHash, _ = utils.GetFrontmatterHash(metaData)

				// Collect Dependencies
				dependencies.Tags = post.Tags
			}

			if post.Draft && !b.cfg.IncludeDrafts {
				return
			}

			cardDestPath := filepath.Join("public", "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp")
			_ = b.DestFs.MkdirAll(filepath.Dir(cardDestPath), 0755)

			if forceSocialRebuild || cachedHash != frontmatterHash {
				socialTasksMu.Lock()
				socialCardTasks = append(socialCardTasks, socialCardTask{
					path:            relPath,
					relPath:         strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
					cardDestPath:    cardDestPath,
					metaData:        metaData,
					frontmatterHash: frontmatterHash,
				})
				socialTasksMu.Unlock()
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

			skipRendering := !shouldForce
			if destInfo, err := os.Stat(destPath); err != nil || !destInfo.ModTime().After(info.ModTime()) {
				skipRendering = false
			}

			if !skipRendering {
				b.rnd.RenderPage(destPath, models.PageData{
					Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
					Meta: metaData, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
					TabTitle: post.Title + " | " + b.cfg.Title, Permalink: post.Link, Image: imagePath,
					HasMath: post.HasMath, HasMermaid: post.HasMermaid, TOC: toc, Config: b.cfg,
				})
				mu.Lock()
				anyPostChanged = true
				mu.Unlock()
			}

			// Prepare data for BatchCommit and Memory Accumulation
			mu.Lock()

			// Accumulate for return
			for _, t := range post.Tags {
				tagMap[strings.ToLower(strings.TrimSpace(t))] = append(tagMap[strings.ToLower(strings.TrimSpace(t))], post)
			}
			if post.Pinned {
				pinnedPosts = append(pinnedPosts, post)
			} else {
				allPosts = append(allPosts, post)
			}
			searchRecord.ID = len(indexedPosts)
			indexedPosts = append(indexedPosts, models.IndexedPost{Record: searchRecord, WordFreqs: wordFreqs, DocLen: docLen})
			mu.Unlock()

			// Cache Store Logic
			if !useCache && b.cacheManager != nil {
				// Store HTML
				htmlHash, _ := b.cacheManager.StoreHTML([]byte(htmlContent))

				postID := cache.GeneratePostID("", relPath)

				// Convert TOC for cache
				var cacheTOC []cache.TOCEntry
				for _, t := range toc {
					cacheTOC = append(cacheTOC, cache.TOCEntry{
						ID:    t.ID,
						Text:  t.Text,
						Level: t.Level,
					})
				}

				newMeta := &cache.PostMeta{
					PostID:      postID,
					Path:        relPath,
					ModTime:     info.ModTime().Unix(),
					ContentHash: frontmatterHash,
					HTMLHash:    htmlHash,
					Title:       post.Title,
					Date:        post.DateObj,
					Tags:        post.Tags,
					ReadingTime: post.ReadingTime,
					Description: post.Description,
					Link:        post.Link,
					Pinned:      post.Pinned,
					Draft:       post.Draft,
					HasMath:     post.HasMath,
					HasMermaid:  post.HasMermaid,
					Meta:        metaData,
					TOC:         cacheTOC,
				}

				newSearch := &cache.SearchRecord{
					Title:    post.Title,
					Tokens:   search.Tokenize(post.Description),
					BM25Data: wordFreqs,
					DocLen:   docLen,
					Content:  plainText,
				}

				newDep := &cache.Dependencies{
					Tags: post.Tags,
				}

				batchMu.Lock()
				newPostsMeta = append(newPostsMeta, newMeta)
				newSearchRecords[postID] = newSearch
				newDeps[postID] = newDep
				batchMu.Unlock()
			}

			if c := atomic.AddInt32(&processedCount, 1); c%10 == 0 || int(c) == len(files) {
				fmt.Printf("   📊 Progress: %d/%d posts processed\n", c, len(files))
			}
		}(path)
	}
	wg.Wait()

	// Commit Batch to BoltDB
	if b.cacheManager != nil && len(newPostsMeta) > 0 {
		if err := b.cacheManager.BatchCommit(newPostsMeta, newSearchRecords, newDeps); err != nil {
			fmt.Printf("⚠️ Failed to commit cache batch: %v\n", err)
		}
	}

	if len(socialCardTasks) > 0 {
		fmt.Printf("🖼️  Generating %d social cards...\n", len(socialCardTasks))
		cardSem := make(chan struct{}, numWorkers)
		for _, t := range socialCardTasks {
			wg.Add(1)
			cardSem <- struct{}{}
			go func(t socialCardTask) {
				defer wg.Done()
				defer func() { <-cardSem }()
				_ = generators.GenerateSocialCard(b.DestFs, b.SourceFs, utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"), utils.GetString(t.metaData, "date"), t.cardDestPath, "static/images/favicon.png", "builder/assets/fonts")
				if b.cacheManager != nil {
					_ = b.cacheManager.SetSocialCardHash(t.path, t.frontmatterHash)
				}
			}(t)
		}
		wg.Wait()
	}

	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)
	return allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404
}

// renderCachedPosts re-renders posts using cached HTML content (skips parsing)
func (b *Builder) renderCachedPosts() {
	if b.cacheManager == nil {
		fmt.Println("⚠️ Cache manager not available, skipping fast render.")
		return
	}

	ids, err := b.cacheManager.ListAllPosts()
	if err != nil {
		fmt.Printf("⚠️ Failed to list posts from cache: %v\n", err)
		return
	}

	fmt.Printf("⚡ Fast-rendering %d posts from cache...\n", len(ids))
	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(postID string) {
			defer wg.Done()
			defer func() { <-sem }()

			cp, err := b.cacheManager.GetPostByID(postID)
			if err != nil || cp == nil {
				return
			}

			htmlBytes, err := b.cacheManager.GetHTMLContent(cp)
			if err != nil || htmlBytes == nil {
				return
			}

			relPath := cp.Path
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
			destPath := filepath.Join("public", htmlRelPath)

			// Determine image path (logic duplicated from processPosts for now)
			imagePath := b.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
			if img, ok := cp.Meta["image"].(string); ok {
				if b.cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = b.cfg.BaseURL + img
			}

			// Convert TOC for PageData
			var toc []models.TOCEntry
			for _, t := range cp.TOC {
				toc = append(toc, models.TOCEntry{
					ID:    t.ID,
					Text:  t.Text,
					Level: t.Level,
				})
			}

			b.rnd.RenderPage(destPath, models.PageData{
				Title: cp.Title, Description: cp.Description, Content: template.HTML(string(htmlBytes)),
				Meta: cp.Meta, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
				TabTitle: cp.Title + " | " + b.cfg.Title, Permalink: cp.Link, Image: imagePath,
				HasMath: cp.HasMath, HasMermaid: cp.HasMermaid, TOC: toc, Config: b.cfg,
			})
		}(id)
	}
	wg.Wait()
}
