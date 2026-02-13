package services

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/cache"
	"my-ssg/builder/config"
	"my-ssg/builder/metrics"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer/native"
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

// Constants for magic numbers
const (
	wordsPerMinute = 120.0 // Average reading speed for calculating reading time
)

// socialCardTask represents a social card generation task
type socialCardTask struct {
	path, relPath, cardDestPath string
	metaData                    map[string]interface{}
	frontmatterHash             string
}

type postServiceImpl struct {
	cfg            *config.Config
	cache          CacheService
	renderer       RenderService
	logger         *slog.Logger
	metrics        *metrics.BuildMetrics
	md             goldmark.Markdown
	nativeRenderer *native.Renderer
	sourceFs       afero.Fs
	destFs         afero.Fs
	diagramAdapter *cache.DiagramCacheAdapter // Kept as specific type or interface?

	// Mutex for D2/Math rendering safety if needed
	mu sync.Mutex
}

func NewPostService(
	cfg *config.Config,
	cacheSvc CacheService,
	renderer RenderService,
	logger *slog.Logger,
	metrics *metrics.BuildMetrics,
	md goldmark.Markdown,
	nativeRenderer *native.Renderer,
	sourceFs, destFs afero.Fs,
	diagramAdapter *cache.DiagramCacheAdapter,
) PostService {
	return &postServiceImpl{
		cfg:            cfg,
		cache:          cacheSvc,
		renderer:       renderer,
		logger:         logger,
		metrics:        metrics,
		md:             md,
		nativeRenderer: nativeRenderer,
		sourceFs:       sourceFs,
		destFs:         destFs,
		diagramAdapter: diagramAdapter,
	}
}

func (s *postServiceImpl) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool) (*PostResult, error) {
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
	_ = afero.Walk(s.sourceFs, s.cfg.ContentDir, func(path string, info fs.FileInfo, err error) error {
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

	cardPool := utils.NewWorkerPool(ctx, numWorkers, func(task socialCardTask) {
		s.generateSocialCard(task)
	})
	cardPool.Start()

	// Phase 0: Load global metadata from cache for complete sidebar/neighbor context
	allMetadataMap := make(map[string]models.PostMetadata)
	if s.cache != nil {
		if Lister, ok := s.cache.(interface{ ListAllPosts() ([]string, error) }); ok {
			ids, _ := Lister.ListAllPosts()
			cachedPosts, _ := s.cache.GetPostsByIDs(ids)
			for _, cp := range cachedPosts {
				allMetadataMap[cp.Link] = models.PostMetadata{
					Title: cp.Title, Link: cp.Link, Weight: cp.Weight, Version: cp.Version,
					DateObj: cp.Date, ReadingTime: cp.ReadingTime, Description: cp.Description,
					Tags: cp.Tags, Pinned: cp.Pinned, Draft: cp.Draft,
				}
			}
		}
	}

	// Mutex for protecting allMetadataMap during concurrent access
	var metadataMu sync.RWMutex

	parsePool := utils.NewWorkerPool(ctx, numWorkers, func(pt struct {
		idx     int
		path    string
		version string
	}) {
		idx, path, version := pt.idx, pt.path, pt.version

		relPath, _ := utils.SafeRel(s.cfg.ContentDir, path)
		htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

		cleanHtmlRelPath := htmlRelPath
		if version != "" {
			cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
		}

		var destPath string
		if version != "" {
			destPath = filepath.Join(s.cfg.OutputDir, version, cleanHtmlRelPath)
		} else {
			destPath = filepath.Join(s.cfg.OutputDir, htmlRelPath)
		}

		// 1. Resolve from Cache
		var cachedMeta *cache.PostMeta
		var cachedSearch *cache.SearchRecord
		var cachedHTML []byte
		var err error
		var info os.FileInfo
		exists := false

		if s.cache != nil {
			cachedMeta, err = s.cache.GetPostByPath(relPath)
			if err == nil && cachedMeta != nil {
				exists = true
				info, _ = s.sourceFs.Stat(path)
				if info != nil && info.ModTime().Unix() > cachedMeta.ModTime {
					exists = false
				}
			}
		}

		useCache := exists && !shouldForce

		var cachedHash string
		if s.cache != nil && !useCache {
			cachedHash, _ = s.cache.GetSocialCardHash(relPath)
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
			cachedHTML, err = s.cache.GetHTMLContent(cachedMeta)
			if err != nil || cachedHTML == nil {
				useCache = false
			} else {
				cachedSearch, err = s.cache.GetSearchRecord(cachedMeta.PostID)
				if err != nil || cachedSearch == nil {
					useCache = false
				}
			}
		}

		if useCache {
			s.metrics.IncrementCacheHit()
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
				Title:           cachedSearch.Title,
				NormalizedTitle: cachedSearch.NormalizedTitle,
				Link:            htmlRelPath,
				Description:     cachedMeta.Description,
				Tags:            cachedMeta.Tags,
				NormalizedTags:  cachedSearch.NormalizedTags,
				Content:         cachedSearch.Content,
				Version:         cachedMeta.Version,
			}
			docLen = cachedSearch.DocLen
			wordFreqs = cachedSearch.BM25Data
		} else {
			s.metrics.IncrementCacheMiss()
			if info == nil {
				info, _ = s.sourceFs.Stat(path)
			}
			source, _ := afero.ReadFile(s.sourceFs, path)

			// Copy raw markdown to output for "View Source" feature
			if s.cfg.Features.RawMarkdown {
				// Use filepath to handle OS-specific path separators correctly
				mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
				_ = s.destFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
				_ = afero.WriteFile(s.destFs, mdDestPath, source, 0644)
			}

			ctx := parser.NewContext()
			ctx.Set(mdParser.ContextKeyFilePath, path)
			docNode := s.md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))

			// Use BufferPool
			buf := utils.SharedBufferPool.Get()
			defer utils.SharedBufferPool.Put(buf)

			_ = s.md.Renderer().Render(buf, source, docNode)
			htmlContent = buf.String()

			if pairs := mdParser.GetD2SVGPairSlice(ctx); pairs != nil {
				htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, pairs)
			}

			var diagramCache map[string]string
			if s.diagramAdapter != nil {
				diagramCache = s.diagramAdapter.AsMap()
			}

			if strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(") {
				htmlContent = mdParser.RenderMathForHTML(htmlContent, s.nativeRenderer, diagramCache, &s.mu)
			}
			if s.cfg.CompressImages {
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

			postLink := utils.BuildURL(s.cfg.BaseURL, version, cleanHtmlRelPath)

			post = models.PostMetadata{
				Title: utils.GetString(metaData, "title"), Link: postLink,
				Description: utils.GetString(metaData, "description"), Tags: utils.GetSlice(metaData, "tags"),
				ReadingTime: int(math.Ceil(float64(wordCount) / wordsPerMinute)), Pinned: isPinned, Weight: weight,
				DateObj: dateObj, Draft: utils.GetBool(metaData, "draft"), Version: version,
			}

			plainText = mdParser.ExtractPlainText(docNode, source)

			// Pre-compute normalized fields for search
			normalizedTags := make([]string, len(post.Tags))
			for i, t := range post.Tags {
				normalizedTags[i] = strings.ToLower(t)
			}

			searchRecord = models.PostRecord{
				Title:           post.Title,
				NormalizedTitle: strings.ToLower(post.Title),
				Link:            htmlRelPath,
				Description:     post.Description,
				Tags:            post.Tags,
				NormalizedTags:  normalizedTags,
				Content:         plainText,
				Version:         version,
			}

			// Use strings.Builder for tokenization pre-processing to reduce allocations
			var sb strings.Builder
			// Estimate size: Title (50) + Desc (150) + Tags (50) + Content (5000)
			sb.Grow(len(searchRecord.Title) + len(searchRecord.Description) + len(searchRecord.Content) + 200)
			sb.WriteString(searchRecord.Title)
			sb.WriteByte(' ')
			sb.WriteString(searchRecord.Description)
			sb.WriteByte(' ')
			for _, t := range searchRecord.Tags {
				sb.WriteString(t)
				sb.WriteByte(' ')
			}
			sb.WriteString(searchRecord.Content)

			words = search.Tokenize(strings.ToLower(sb.String()))
			docLen = len(words)
			wordFreqs = make(map[string]int)
			for _, w := range words {
				if len(w) >= 2 {
					wordFreqs[w]++
				}
			}
			frontmatterHash, _ = utils.GetFrontmatterHash(metaData)
		}

		if post.Draft && !s.cfg.IncludeDrafts {
			return
		}

		cardDestPath := filepath.ToSlash(filepath.Join(s.cfg.OutputDir, "static", "images", "cards", strings.TrimSuffix(htmlRelPath, ".html")+".webp"))
		if err := s.destFs.MkdirAll(filepath.Dir(cardDestPath), 0755); err != nil {
			s.logger.Error("Failed to create social card directory", "path", filepath.Dir(cardDestPath), "error", err)
		}

		// Check if card exists in destFs (virtual filesystem), not OS filesystem
		cardExists := false
		if info, err := s.destFs.Stat(cardDestPath); err == nil && !info.IsDir() {
			if sourceInfo, err := s.sourceFs.Stat(path); err == nil {
				if info.ModTime().After(sourceInfo.ModTime()) {
					cardExists = true
				}
			}
		}

		if forceSocialRebuild || (cachedHash != frontmatterHash || !cardExists) {
			// s.logger.Debug("Submitting social card task", "path", relPath, "cardDestPath", cardDestPath, "forceSocialRebuild", forceSocialRebuild, "cachedHash", cachedHash, "frontmatterHash", frontmatterHash, "cardExists", cardExists)
			cardPool.Submit(socialCardTask{
				path:            relPath,
				relPath:         strings.TrimSuffix(htmlRelPath, ".html") + ".webp",
				cardDestPath:    cardDestPath,
				metaData:        metaData,
				frontmatterHash: frontmatterHash,
			})
		} else if cardExists {
			if s.cache != nil && cachedHash == "" {
				_ = s.cache.SetSocialCardHash(relPath, frontmatterHash)
			}
		}

		imagePath := s.cfg.BaseURL + "/static/images/cards/" + strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
		if img, ok := metaData["image"].(string); ok {
			if s.cfg.CompressImages && !strings.HasPrefix(img, "http") {
				ext := filepath.Ext(img)
				if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
					img = img[:len(img)-len(ext)] + ".webp"
				}
			}
			imagePath = s.cfg.BaseURL + img
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
				info, _ = s.sourceFs.Stat(path)
			}
			if destInfo, err := os.Stat(destPath); err != nil || !destInfo.ModTime().After(info.ModTime()) {
				willRender = true
			}
		}

		// Copy raw markdown to output for "View Source" feature (for cached posts too)
		if s.cfg.Features.RawMarkdown {
			mdDestPath := destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".md"
			if _, err := os.Stat(mdDestPath); os.IsNotExist(err) {
				sourceBytes, _ := afero.ReadFile(s.sourceFs, path)
				if len(sourceBytes) > 0 {
					_ = s.destFs.MkdirAll(filepath.Dir(mdDestPath), 0755)
					_ = afero.WriteFile(s.destFs, mdDestPath, sourceBytes, 0644)
				}
			}
		}

		if willRender {
			renderQueue[idx] = RenderContext{
				DestPath: destPath,
				Version:  version,
				Data: models.PageData{
					Title: post.Title, Description: post.Description, Content: template.HTML(htmlContent),
					Meta: metaData, BaseURL: s.cfg.BaseURL, BuildVersion: s.cfg.BuildVersion,
					TabTitle: post.Title + " | " + s.cfg.Title, Permalink: post.Link, Image: imagePath,
					TOC: toc, Config: s.cfg,
					CurrentVersion: version,
					IsOutdated:     version != "",
					Versions:       s.cfg.GetVersionsMetadata(version),
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

		if !useCache && s.cache != nil {
			postID := cache.GeneratePostID("", relPath)
			newMeta := &cache.PostMeta{
				PostID: postID, Path: relPath, ModTime: info.ModTime().Unix(),
				ContentHash: frontmatterHash, Title: post.Title, Date: post.DateObj,
				Tags: post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
				Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
				Meta: metaData, TOC: toc, Version: version,
			}
			_ = s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent))
			newSearch := &cache.SearchRecord{
				Title: post.Title, NormalizedTitle: searchRecord.NormalizedTitle,
				BM25Data: wordFreqs, DocLen: docLen, Content: plainText,
				NormalizedTags: searchRecord.NormalizedTags,
			}
			newDep := &cache.Dependencies{Tags: post.Tags}

			batchMu.Lock()
			newPostsMeta = append(newPostsMeta, newMeta)
			newSearchRecords[postID] = newSearch
			newDeps[postID] = newDep
			batchMu.Unlock()
		}

		s.metrics.IncrementPostsProcessed()
		_ = atomic.AddInt32(&processedCount, 1)
	})
	parsePool.Start()

	for i, path := range files {
		parsePool.Submit(struct {
			idx     int
			path    string
			version string
		}{i, path, fileVersions[i]})
	}
	parsePool.Stop()
	cardPool.Stop() // Wait for all social card generation to complete

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

	renderPool := utils.NewWorkerPool(ctx, numWorkers, func(t RenderContext) {
		t.Data.SiteTree = siteTrees[t.Version]
		s.renderer.RenderPage(t.DestPath, t.Data)
	})
	renderPool.Start()

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

		renderPool.Submit(*task)
	}
	renderPool.Stop()

	if s.cache != nil && len(newPostsMeta) > 0 {
		if err := s.cache.BatchCommit(newPostsMeta, newSearchRecords, newDeps); err != nil {
			s.logger.Warn("Failed to commit cache batch", "error", err)
		}
	}

	runtime.GC()

	// Sort posts to ensure consistent ordering
	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	return &PostResult{
		AllPosts:       allPosts,
		PinnedPosts:    pinnedPosts,
		TagMap:         tagMap,
		IndexedPosts:   indexedPosts,
		AnyPostChanged: anyPostChanged,
		Has404:         has404,
	}, nil
}
