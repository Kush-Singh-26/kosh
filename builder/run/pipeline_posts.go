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

	type socialCardTask struct {
		path, relPath, cardDestPath string
		metaData                    map[string]interface{}
		frontmatterHash             string
	}
	var socialCardTasks []socialCardTask
	var socialTasksMu sync.Mutex

	var files []string
	afero.Walk(b.SourceFs, "content", func(path string, info fs.FileInfo, err error) error {
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

			cacheKey := utils.NormalizeCacheKey(path)
			b.mu.Lock()
			cached, exists := b.buildCache.Posts[cacheKey]
			b.mu.Unlock()

			// Get cached social card hash from BoltDB
			var cachedHash string
			if b.cacheManager != nil {
				cachedHash, _ = b.cacheManager.GetSocialCardHash(relPath)
			}

			useCache := exists && !shouldForce && !info.ModTime().After(cached.ModTime)

			var htmlContent string
			var metaData map[string]interface{}
			var post models.PostMetadata
			var searchRecord models.PostRecord
			var wordFreqs map[string]int
			var docLen int
			var toc []models.TOCEntry
			var frontmatterHash string

			if useCache {
				htmlContent, metaData, post, searchRecord, wordFreqs, docLen, toc, frontmatterHash = cached.HTMLContent, cached.Meta, cached.Metadata, cached.SearchRecord, cached.WordFreqs, cached.DocLen, cached.TOC, cached.FrontmatterHash
				post.Link = b.cfg.BaseURL + "/" + htmlRelPath
			} else {
				source, _ := afero.ReadFile(b.SourceFs, path)
				ctx := parser.NewContext()
				docNode := b.md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))
				var buf bytes.Buffer
				_ = b.md.Renderer().Render(&buf, source, docNode)
				htmlContent = buf.String()

				if pairs := mdParser.GetD2SVGPairSlice(ctx); pairs != nil {
					htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
				}
				if strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(") {
					htmlContent = mdParser.RenderMathForHTML(htmlContent, b.native, b.buildCache.DiagramCache, &b.mu)
				}
				if b.cfg.CompressImages {
					htmlContent = utils.ReplaceToWebP(htmlContent)
				}

				metaData = meta.Get(ctx)
				dateStr := utils.GetString(metaData, "date")
				dateObj, _ := time.Parse("2006-01-02", dateStr)
				isPinned, _ := metaData["pinned"].(bool)
				wordCount := len(strings.Fields(string(source)))

				post = models.PostMetadata{
					Title: utils.GetString(metaData, "title"), Link: b.cfg.BaseURL + "/" + htmlRelPath,
					Description: utils.GetString(metaData, "description"), Tags: utils.GetSlice(metaData, "tags"),
					ReadingTime: int(math.Ceil(float64(wordCount) / 120.0)), Pinned: isPinned,
					DateObj: dateObj, HasMath: strings.Contains(string(source), "$"), HasMermaid: mdParser.HasD2(ctx),
				}
				searchRecord = models.PostRecord{Title: post.Title, Link: htmlRelPath, Description: post.Description, Tags: post.Tags, Content: mdParser.ExtractPlainText(docNode, source)}
				words := search.Tokenize(strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content))
				docLen = len(words)
				wordFreqs = make(map[string]int)
				for _, w := range words {
					if len(w) >= 2 {
						wordFreqs[w]++
					}
				}
				frontmatterHash, _ = utils.GetFrontmatterHash(metaData)
			}

			if isDraft, _ := metaData["draft"].(bool); isDraft && !b.cfg.IncludeDrafts {
				return
			}

			cardDestPath := filepath.Join("public", "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp")
			b.DestFs.MkdirAll(filepath.Dir(cardDestPath), 0755)

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

			mu.Lock()
			if !useCache {
				b.buildCache.Posts[cacheKey] = models.CachedPost{ModTime: info.ModTime(), FrontmatterHash: frontmatterHash, Metadata: post, SearchRecord: searchRecord, WordFreqs: wordFreqs, DocLen: docLen, HTMLContent: htmlContent, TOC: toc, Meta: metaData, HasMermaid: post.HasMermaid}
			}
			for _, t := range post.Tags {
				b.buildCache.Dependencies.Tags["tag:"+strings.ToLower(strings.TrimSpace(t))] = append(b.buildCache.Dependencies.Tags["tag:"+strings.ToLower(strings.TrimSpace(t))], path)
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

			if c := atomic.AddInt32(&processedCount, 1); c%10 == 0 || int(c) == len(files) {
				fmt.Printf("   📊 Progress: %d/%d posts processed\n", c, len(files))
			}
		}(path)
	}
	wg.Wait()

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
					b.cacheManager.SetSocialCardHash(t.path, t.frontmatterHash)
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
	b.mu.Lock()
	posts := b.buildCache.Posts
	b.mu.Unlock()

	fmt.Printf("⚡ Fast-rendering %d posts from cache...\n", len(posts))
	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for key, cached := range posts {
		wg.Add(1)
		sem <- struct{}{}
		go func(k string, cp models.CachedPost) {
			defer wg.Done()
			defer func() { <-sem }()

			relPath, _ := filepath.Rel("content", k)
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

			b.rnd.RenderPage(destPath, models.PageData{
				Title: cp.Metadata.Title, Description: cp.Metadata.Description, Content: template.HTML(cp.HTMLContent),
				Meta: cp.Meta, BaseURL: b.cfg.BaseURL, BuildVersion: b.cfg.BuildVersion,
				TabTitle: cp.Metadata.Title + " | " + b.cfg.Title, Permalink: cp.Metadata.Link, Image: imagePath,
				HasMath: cp.Metadata.HasMath, HasMermaid: cp.Metadata.HasMermaid, TOC: cp.TOC, Config: b.cfg,
			})
		}(key, cached)
	}
	wg.Wait()
}
