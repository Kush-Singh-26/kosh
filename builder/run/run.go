package run

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/config"
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer"
	"my-ssg/builder/renderer/headless"
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

// Builder maintains the state for site builds
type Builder struct {
	cfg             *config.Config
	socialCardCache *utils.SocialCardCache
	buildCache      *models.MetadataCache
	md              goldmark.Markdown
	rnd             *renderer.Renderer
	headless        *headless.Orchestrator
	mu              sync.Mutex
	staticServer    *http.Server
	staticPort      int
	staticCancel    context.CancelFunc
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	utils.InitMinifier()

	// Create cache directory if it doesn't exist
	_ = os.MkdirAll(".kosh-cache", 0755)

	// Load caches from separate directory (not deployed to production)
	socialCardCache, cacheErr := utils.LoadSocialCardCache(".kosh-cache/.social-card-cache.json")
	if cacheErr != nil {
		fmt.Printf("Warning: Failed to load social card cache: %v\n", cacheErr)
		socialCardCache = utils.NewSocialCardCache()
	}

	buildCache, cacheErr := utils.LoadBuildCache(".kosh-cache/.kosh-build-cache.json")
	if cacheErr != nil {
		fmt.Printf("Warning: Failed to load build cache: %v\n", cacheErr)
		buildCache = &models.MetadataCache{
			Posts: make(map[string]models.CachedPost),
		}
	}

	// Smart Cache Invalidation: If BaseURL changed, force rebuild everything
	if buildCache.BaseURL != cfg.BaseURL {
		fmt.Printf("🔄 BaseURL changed (%s -> %s). Forcing full rebuild to update asset paths.\n", buildCache.BaseURL, cfg.BaseURL)
		cfg.ForceRebuild = true
		buildCache.BaseURL = cfg.BaseURL
	}

	// Initialize diagram cache if nil
	if buildCache.DiagramCache == nil {
		buildCache.DiagramCache = make(map[string]string)
	}

	// Create headless renderer (lazy start - Chrome only launches when needed)
	headlessRenderer := headless.New()

	builder := &Builder{
		cfg:             cfg,
		socialCardCache: socialCardCache,
		buildCache:      buildCache,
		md:              mdParser.New(cfg.BaseURL, headlessRenderer, buildCache.DiagramCache, &sync.Mutex{}),
		rnd:             renderer.New(cfg.CompressImages),
		headless:        headlessRenderer,
	}

	// Start static asset server for offline rendering
	builder.StartStaticServer()

	return builder
}

// SetDevMode enables/disables development mode (affects CSS hashing)
func (b *Builder) SetDevMode(isDev bool) {
	b.cfg.IsDev = isDev
}

// SaveCaches persists the build and social card caches to disk
func (b *Builder) SaveCaches() {
	// Ensure cache directory exists
	_ = os.MkdirAll(".kosh-cache", 0755)

	if saveErr := utils.SaveSocialCardCache(".kosh-cache/.social-card-cache.json", b.socialCardCache); saveErr != nil {
		fmt.Printf("Warning: Failed to save social card cache: %v\n", saveErr)
	}
	if saveErr := utils.SaveBuildCache(".kosh-cache/.kosh-build-cache.json", b.buildCache); saveErr != nil {
		fmt.Printf("Warning: Failed to save build cache: %v\n", saveErr)
	}
	fmt.Printf("   💾 Saved build cache to .kosh-cache/\n")
}

// StartStaticServer starts a temporary HTTP server to serve static assets for headless rendering
func (b *Builder) StartStaticServer() {
	// Try ports: 31415 (π), 31416, 31417, then random
	ports := []int{31415, 31416, 31417, 0}

	for _, port := range ports {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if port != 0 {
				log.Printf("⚠️ Port %d in use, trying next...", port)
			}
			continue
		}

		// Get the actual port (in case port was 0)
		actualPort := listener.Addr().(*net.TCPAddr).Port
		b.staticPort = actualPort

		// Create file server for static directory
		fileServer := http.FileServer(http.Dir("static"))
		mux := http.NewServeMux()
		mux.Handle("/", http.StripPrefix("/", fileServer))

		// Create server with context for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		b.staticCancel = cancel
		b.staticServer = &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", actualPort),
			Handler: mux,
		}

		// Start server in goroutine
		go func() {
			if err := b.staticServer.Serve(listener); err != nil && err != http.ErrServerClosed {
				log.Printf("⚠️ Static server error: %v", err)
			}
		}()

		// Wait for server to be ready (health check)
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
		b.headless.SetBaseURL(baseURL)

		// Quick health check with timeout
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer healthCancel()

		for {
			select {
			case <-healthCtx.Done():
				log.Printf("⚠️ Static server health check timeout, using CDN fallback")
				return
			default:
				// Try to connect
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/css/katex.min.css", actualPort))
				if err == nil {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						log.Printf("🌐 Static asset server ready on %s", baseURL)

						// Close listener when context is cancelled
						go func() {
							<-ctx.Done()
							listener.Close()
						}()

						return
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	log.Println("⚠️ Could not start static server, falling back to CDN")
}

// StopStaticServer gracefully shuts down the temporary static server
func (b *Builder) StopStaticServer() {
	if b.staticCancel != nil {
		b.staticCancel()
	}
	if b.staticServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.staticServer.Shutdown(ctx); err != nil {
			log.Printf("⚠️ Error shutting down static server: %v", err)
		}
		log.Println("🌐 Static asset server stopped")
	}
}

// Run executes the main build logic (one-off build - stops static server after)
func Run(args []string) {
	b := NewBuilder(args)
	defer b.headless.Stop()    // Cleanup Chrome on exit
	defer b.StopStaticServer() // Stop static asset server on exit
	defer b.SaveCaches()
	b.Build()
}

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	var affected []string

	switch templatePath {
	case "templates/layout.html":
		// Layout affects ALL posts - return nil to indicate full rebuild needed
		return nil
	case "templates/index.html":
		// Index template only affects index pages (not individual posts)
		// Return empty slice - no posts need rebuilding
		return []string{}
	case "templates/404.html":
		// 404 only affects the 404 page
		return []string{}
	case "templates/graph.html":
		// Graph only affects the knowledge graph page
		return []string{}
	case "static/css/layout.css", "static/css/theme.css":
		// CSS changes affect all posts - return nil for full rebuild
		return nil
	case "kosh.yaml":
		// Config changes might affect all posts - return nil
		return nil
	case "builder/generators/pwa.go":
		// SW generator changes - need to regenerate SW
		return []string{}
	default:
		// Unknown dependency - return nil to be safe
		return nil
	}

	return affected
}

// BuildChanged rebuilds only the changed file (for watch mode)
func (b *Builder) BuildChanged(changedPath string) {
	// If it's a markdown file, do partial rebuild
	if strings.HasSuffix(changedPath, ".md") && strings.HasPrefix(changedPath, "content") {
		fmt.Printf("⚡ Quick rebuild for: %s\n", changedPath)
		b.buildSinglePost(changedPath)
		return
	}

	// For templates, static files, or config - do full rebuild
	fmt.Printf("⚡ Full rebuild needed for: %s\n", changedPath)
	b.Build()
	b.SaveCaches()
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Builder) buildSinglePost(path string) {
	// Read the changed file to check what actually changed
	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		b.Build() // Fall back to full rebuild on error
		return
	}

	// Parse frontmatter to get new hash
	context := parser.NewContext()
	reader := text.NewReader(source)
	b.md.Parser().Parse(reader, parser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	// Check if we have a cached version and compare frontmatter hashes
	cacheKey := utils.NormalizeCacheKey(path)
	b.mu.Lock()
	cached, exists := b.buildCache.Posts[cacheKey]
	b.mu.Unlock()

	if exists && cached.FrontmatterHash == newFrontmatterHash {
		// Only content changed (not frontmatter) - do lightweight rebuild
		fmt.Printf("   📝 Content-only change detected. Fast rebuild...\n")
		b.buildContentOnly(path)
		b.SaveCaches()
	} else {
		// Frontmatter changed (or no cache) - need full rebuild for global pages
		if exists {
			fmt.Printf("   🔄 Frontmatter changed. Full rebuild needed...\n")
		}
		// Invalidate cache for this post
		b.mu.Lock()
		delete(b.buildCache.Posts, cacheKey)
		b.mu.Unlock()
		// Full rebuild
		b.Build()
		b.SaveCaches()
	}
}

// buildContentOnly rebuilds just a single post's HTML without regenerating global pages
// This is used when only the content changed (not frontmatter) for fast dev iteration
func (b *Builder) buildContentOnly(path string) {
	cfg := b.cfg

	// Read and parse the markdown file
	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		return
	}

	info, _ := os.Stat(path)
	relPath, _ := filepath.Rel("content", path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	destPath := filepath.Join("public", htmlRelPath)
	fullLink := cfg.BaseURL + "/" + htmlRelPath

	// Parse markdown
	context := parser.NewContext()
	reader := text.NewReader(source)
	docNode := b.md.Parser().Parse(reader, parser.WithContext(context))

	var buf bytes.Buffer
	if err := b.md.Renderer().Render(&buf, source, docNode); err != nil {
		fmt.Printf("   ❌ Error rendering %s: %v\n", path, err)
		return
	}
	htmlContent := buf.String()

	// Post-process: mermaid diagrams with dual-theme support
	if orderedPairs := mdParser.GetMermaidSVGPairSlice(context); orderedPairs != nil {
		htmlContent = mdParser.ReplaceMermaidBlocksWithThemeSupport(htmlContent, orderedPairs)
	}

	// Post-process: math expressions
	hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")
	if hasMath {
		htmlContent = mdParser.RenderMathForHTML(htmlContent, b.headless, b.buildCache.DiagramCache, &b.mu)
	}

	if cfg.CompressImages {
		htmlContent = utils.ReplaceToWebP(htmlContent)
	}

	metaData := meta.Get(context)
	plainText := mdParser.ExtractPlainText(docNode, source)
	wordCount := len(strings.Fields(string(source)))
	readTime := int(math.Ceil(float64(wordCount) / 120.0))
	isPinned, _ := metaData["pinned"].(bool)
	dateStr := utils.GetString(metaData, "date")
	dateObj, _ := time.Parse("2006-01-02", dateStr)
	isDraft, _ := metaData["draft"].(bool)

	toc := mdParser.GetTOC(context)
	hasMermaid := mdParser.HasMermaid(context)

	post := models.PostMetadata{
		Title:       utils.GetString(metaData, "title"),
		Link:        fullLink,
		Description: utils.GetString(metaData, "description"),
		Tags:        utils.GetSlice(metaData, "tags"),
		ReadingTime: readTime,
		Pinned:      isPinned,
		Draft:       isDraft,
		DateObj:     dateObj,
		HasMath:     hasMath,
		HasMermaid:  hasMermaid,
	}

	searchRecord := models.PostRecord{
		Title:       post.Title,
		Link:        htmlRelPath,
		Description: post.Description,
		Tags:        post.Tags,
		Content:     plainText,
	}

	// Pre-compute word frequencies for search
	fullText := strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content)
	words := search.Tokenize(fullText)
	docLen := len(words)
	wordFreqs := make(map[string]int)
	for _, word := range words {
		if len(word) < 2 {
			continue
		}
		wordFreqs[word]++
	}

	frontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	// Update cache with normalized key
	cacheKey := utils.NormalizeCacheKey(path)
	b.mu.Lock()
	b.buildCache.Posts[cacheKey] = models.CachedPost{
		ModTime:         info.ModTime(),
		FrontmatterHash: frontmatterHash,
		Metadata:        post,
		SearchRecord:    searchRecord,
		WordFreqs:       wordFreqs,
		DocLen:          docLen,
		HTMLContent:     htmlContent,
		TOC:             toc,
		Meta:            metaData,
		HasMermaid:      hasMermaid,
	}
	b.mu.Unlock()
	_ = hasMath

	// Skip drafts if not included
	if isDraft && !cfg.IncludeDrafts {
		fmt.Printf("   ⏩ Skipping draft: %s\n", relPath)
		return
	}

	// Get image path
	cardRelPath := strings.TrimSuffix(htmlRelPath, ".html") + ".webp"
	imagePath := cfg.BaseURL + "/static/images/cards/" + cardRelPath
	if img, ok := metaData["image"].(string); ok {
		if cfg.CompressImages && !strings.HasPrefix(img, "http") {
			ext := filepath.Ext(img)
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
				img = img[:len(img)-len(ext)] + ".webp"
			}
		}
		imagePath = cfg.BaseURL + img
	}

	// Render the post HTML
	fmt.Printf("   Rendering: %s\n", htmlRelPath)
	b.rnd.RenderPage(destPath, models.PageData{
		Title:        post.Title,
		Description:  post.Description,
		Content:      template.HTML(htmlContent),
		Meta:         metaData,
		BaseURL:      cfg.BaseURL,
		BuildVersion: cfg.BuildVersion,
		TabTitle:     post.Title + " | " + cfg.Title,
		Permalink:    fullLink,
		Image:        imagePath,
		HasMath:      hasMath,
		HasMermaid:   hasMermaid,
		TOC:          toc,
		Config:       cfg,
	})

	fmt.Printf("   ✅ Content-only rebuild complete for: %s\n", htmlRelPath)
}

// Build executes a single build pass
func (b *Builder) Build() {
	cfg := b.cfg
	numWorkers := runtime.NumCPU()
	fmt.Printf("🔨 Building site... (Version: %d) | Parallel Workers: %d\n", cfg.BuildVersion, numWorkers)

	// Initialize template mod times tracking if needed
	if b.buildCache.TemplateModTimes == nil {
		b.buildCache.TemplateModTimes = make(map[string]time.Time)
	}

	// Check dependencies for force rebuild
	globalDependencies := []string{"templates/layout.html", "templates/index.html", "templates/404.html", "templates/graph.html", "static/css/layout.css", "static/css/theme.css", "kosh.yaml", "builder/generators/pwa.go"}
	forceSocialRebuild := false
	shouldForce := b.cfg.ForceRebuild
	var affectedPosts []string

	if indexInfo, err := os.Stat("public/index.html"); err == nil {
		lastBuildTime := indexInfo.ModTime()

		for _, dep := range globalDependencies {
			if info, err := os.Stat(dep); err == nil && info.ModTime().After(lastBuildTime) {
				fmt.Printf("⚡ Global change detected in [%s].\n", dep)

				// Granular invalidation based on which template changed
				affected := b.invalidateForTemplate(dep)
				if affected != nil {
					affectedPosts = append(affectedPosts, affected...)
					fmt.Printf("   🔄 Invalidated %d posts affected by %s\n", len(affected), filepath.Base(dep))
				} else {
					// No specific posts identified, force full rebuild
					shouldForce = true
				}
			}
		}

		if info, err := os.Stat("builder/generators/social.go"); err == nil && info.ModTime().After(lastBuildTime) {
			fmt.Println("⚡ Social generator change detected. Forcing social card rebuild.")
			forceSocialRebuild = true
		}
	} else {
		shouldForce = true
		forceSocialRebuild = true
	}

	// Update template mod times in cache
	for _, dep := range globalDependencies {
		if info, err := os.Stat(dep); err == nil {
			b.buildCache.TemplateModTimes[dep] = info.ModTime()
		}
	}

	// Check if Service Worker needs regeneration
	swPath := "public/sw.js"
	if swInfo, err := os.Stat(swPath); err == nil {
		// Regenerate SW if pwa.go template is newer
		if pwaInfo, pwaErr := os.Stat("builder/generators/pwa.go"); pwaErr == nil && pwaInfo.ModTime().After(swInfo.ModTime()) {
			fmt.Printf("⚡ Service Worker template updated. Regenerating...\n")
			shouldForce = true
		}
	}

	// Reset global ForceRebuild so it doesn't stick in watch mode
	b.cfg.ForceRebuild = false

	b.md = mdParser.New(cfg.BaseURL, b.headless, b.buildCache.DiagramCache, &b.mu)
	b.rnd = renderer.New(cfg.CompressImages)

	_ = os.MkdirAll("public/tags", 0755)
	_ = os.MkdirAll("public/static/images/cards", 0755)
	_ = os.MkdirAll("public/sitemap", 0755)

	// Start static processing in background - don't block on it
	go func() {
		if _, err := os.Stat("static"); err == nil {
			_ = utils.CopyDir("static", "public/static", cfg.CompressImages)
		}

		// Process Assets (CSS/JS minification & hashing)
		assets, assetErr := utils.ProcessAssets("static", "public/static")
		if assetErr != nil {
			fmt.Printf("⚠️ Failed to process assets: %v\n", assetErr)
		} else {
			if len(assets) > 0 {
				fmt.Printf("🎨 Processed %d assets\n", len(assets))
			}
		}
		b.rnd.SetAssets(assets)
	}()

	if err := os.WriteFile("public/.nojekyll", []byte(""), 0644); err != nil {
		fmt.Printf("⚠️ Failed to create .nojekyll: %v\n", err)
	}

	fontsDir := "builder/assets/fonts"
	faviconPath := "static/images/favicon.png"

	// Invalidate affected posts from granular template changes
	if len(affectedPosts) > 0 {
		b.mu.Lock()
		for _, postPath := range affectedPosts {
			cacheKey := utils.NormalizeCacheKey(postPath)
			if _, exists := b.buildCache.Posts[cacheKey]; exists {
				delete(b.buildCache.Posts, cacheKey)
			}
		}
		b.mu.Unlock()
		fmt.Printf("🔄 Granular rebuild: %d posts invalidated\n", len(affectedPosts))
	}

	// --- PARALLELIZATION START ---

	// 1. Shared State
	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		indexedPosts   []models.IndexedPost
		tagMap         = make(map[string][]models.PostMetadata)
		has404         bool
		anyPostChanged bool
	)

	// Social card generation tasks (collected for parallel processing)
	type socialCardTask struct {
		path         string
		relPath      string
		cardDestPath string
		metaData     map[string]interface{}
	}
	var socialCardTasks []socialCardTask
	var socialTasksMu sync.Mutex

	// 2. Collect all files first
	var filesToProcess []string
	err := filepath.Walk("content", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".md") || strings.Contains(path, "_index.md") {
			return nil
		}
		if strings.Contains(path, "404.md") {
			b.mu.Lock()
			has404 = true
			b.mu.Unlock()
			return nil
		}
		filesToProcess = append(filesToProcess, path)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	// 2.5 Pre-render all LaTeX math expressions in one batch
	// This eliminates multiple Chrome tabs (one per post) by rendering everything upfront
	if len(filesToProcess) > 0 {
		if err := mdParser.PreRenderMathForAllPosts(filesToProcess, b.headless, b.buildCache.DiagramCache, &b.mu); err != nil {
			fmt.Printf("⚠️  Pre-rendering math failed: %v\n", err)
		}
	}

	// 3. Process files concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, numWorkers) // Semaphore to limit concurrency
	totalFiles := len(filesToProcess)
	var processedCount int32

	if totalFiles > 0 {
		fmt.Printf("📝 Processing %d posts...\n", totalFiles)
	} else {
		fmt.Printf("📝 No posts to process\n")
	}

	for _, path := range filesToProcess {
		wg.Add(1)
		sem <- struct{}{} // Acquire token

		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }() // Release token

			// 1. Basic info
			info, _ := os.Stat(path)
			relPath, _ := filepath.Rel("content", path)
			htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
			relPathNoExt := strings.TrimSuffix(htmlRelPath, ".html")
			destPath := filepath.Join("public", htmlRelPath)
			fullLink := cfg.BaseURL + "/" + htmlRelPath

			// 2. Cache Check
			cacheKey := utils.NormalizeCacheKey(path)
			b.mu.Lock()
			cached, exists := b.buildCache.Posts[cacheKey]
			b.mu.Unlock()

			useCache := exists && !shouldForce && (info.ModTime().Equal(cached.ModTime) || info.ModTime().Before(cached.ModTime))

			var htmlContent string
			var metaData map[string]interface{}
			var post models.PostMetadata
			var searchRecord models.PostRecord
			var wordFreqs map[string]int
			var docLen int
			var toc []models.TOCEntry

			if useCache {
				htmlContent = cached.HTMLContent
				metaData = cached.Meta
				post = cached.Metadata
				searchRecord = cached.SearchRecord
				wordFreqs = cached.WordFreqs
				docLen = cached.DocLen
				toc = cached.TOC

				// Update the link to use the current BaseURL (prevents cache portability issues)
				post.Link = cfg.BaseURL + "/" + htmlRelPath
			} else {
				// 3. Full Parse
				source, _ := os.ReadFile(path)
				var buf bytes.Buffer
				context := parser.NewContext()
				reader := text.NewReader(source)
				docNode := b.md.Parser().Parse(reader, parser.WithContext(context))
				plainText := mdParser.ExtractPlainText(docNode, source)

				if err := b.md.Renderer().Render(&buf, source, docNode); err != nil {
					log.Printf("Error rendering %s: %v", path, err)
					return
				}
				htmlContent = buf.String()

				// Post-process: Replace mermaid code blocks with rendered SVGs (dual-theme)
				if orderedPairs := mdParser.GetMermaidSVGPairSlice(context); orderedPairs != nil {
					htmlContent = mdParser.ReplaceMermaidBlocksWithThemeSupport(htmlContent, orderedPairs)
				}

				// Post-process: Render LaTeX math expressions with KaTeX SSR
				if hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\("); hasMath {
					htmlContent = mdParser.RenderMathForHTML(htmlContent, b.headless, b.buildCache.DiagramCache, &b.mu)
				}

				if cfg.CompressImages {
					htmlContent = utils.ReplaceToWebP(htmlContent)
				}
				metaData = meta.Get(context)

				wordCount := len(strings.Fields(string(source)))
				readTime := int(math.Ceil(float64(wordCount) / 120.0))
				isPinned, _ := metaData["pinned"].(bool)
				dateStr := utils.GetString(metaData, "date")
				dateObj, _ := time.Parse("2006-01-02", dateStr)
				hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")
				hasMermaid := mdParser.HasMermaid(context)
				toc = mdParser.GetTOC(context)

				post = models.PostMetadata{
					Title:       utils.GetString(metaData, "title"),
					Link:        fullLink,
					Description: utils.GetString(metaData, "description"),
					Tags:        utils.GetSlice(metaData, "tags"),
					ReadingTime: readTime,
					Pinned:      isPinned,
					DateObj:     dateObj,
					HasMath:     hasMath,
					HasMermaid:  hasMermaid,
				}

				searchRecord = models.PostRecord{
					Title:       post.Title,
					Link:        htmlRelPath,
					Description: post.Description,
					Tags:        post.Tags,
					Content:     plainText,
				}

				// Pre-compute word frequencies for incremental search indexing
				fullText := strings.ToLower(searchRecord.Title + " " + searchRecord.Description + " " + strings.Join(searchRecord.Tags, " ") + " " + searchRecord.Content)
				words := search.Tokenize(fullText)
				docLen = len(words)
				wordFreqs = make(map[string]int)
				for _, word := range words {
					if len(word) < 2 {
						continue
					}
					wordFreqs[word]++
				}

				// Calculate frontmatter hash for change detection
				frontmatterHash, _ := utils.GetFrontmatterHash(metaData)

				// Update Cache with normalized key
				cacheKey := utils.NormalizeCacheKey(path)
				b.mu.Lock()
				b.buildCache.Posts[cacheKey] = models.CachedPost{
					ModTime:         info.ModTime(),
					FrontmatterHash: frontmatterHash,
					Metadata:        post,
					SearchRecord:    searchRecord,
					WordFreqs:       wordFreqs,
					DocLen:          docLen,
					HTMLContent:     htmlContent,
					TOC:             toc,
					Meta:            metaData,
				}
				b.mu.Unlock()
			}

			// 4. Draft Check
			isDraft, _ := metaData["draft"].(bool)
			if isDraft && !cfg.IncludeDrafts {
				fmt.Printf("⏩ Skipping draft: %s\n", relPath)
				return
			}
			if isDraft {
				fmt.Printf("📝 Including draft: %s\n", relPath)
			}

			// 5. Social Card Logic - Collect tasks for parallel processing
			cardRelPath := relPathNoExt + ".webp"
			cardDestPath := filepath.Join("public", "static", "images", "cards", cardRelPath)
			_ = os.MkdirAll(filepath.Dir(cardDestPath), 0755)

			genCard := false
			b.mu.Lock()
			cachedHash := b.socialCardCache.Hashes[relPath]
			b.mu.Unlock()

			if forceSocialRebuild {
				genCard = true
			} else {
				_, cardExists := os.Stat(cardDestPath)
				if os.IsNotExist(cardExists) {
					genCard = true
				} else {
					frontmatterHash, hashErr := utils.GetFrontmatterHash(metaData)
					if hashErr != nil {
						genCard = true
					} else if cachedHash != frontmatterHash {
						genCard = true
					}
				}
			}

			if genCard {
				// Collect task for parallel processing later
				socialTasksMu.Lock()
				socialCardTasks = append(socialCardTasks, socialCardTask{
					path:         relPath,
					relPath:      cardRelPath,
					cardDestPath: cardDestPath,
					metaData:     metaData,
				})
				socialTasksMu.Unlock()
			} else if frontmatterHash, hashErr := utils.GetFrontmatterHash(metaData); hashErr == nil {
				b.mu.Lock()
				b.socialCardCache.Hashes[relPath] = frontmatterHash
				b.mu.Unlock()
			}

			imagePath := cfg.BaseURL + "/static/images/cards/" + cardRelPath
			if img, ok := metaData["image"].(string); ok {
				if cfg.CompressImages && !strings.HasPrefix(img, "http") {
					ext := filepath.Ext(img)
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						img = img[:len(img)-len(ext)] + ".webp"
					}
				}
				imagePath = cfg.BaseURL + img
			}

			// 6. Rendering
			skipRendering := false
			if !shouldForce {
				if destInfo, err := os.Stat(destPath); err == nil {
					if destInfo.ModTime().After(info.ModTime()) {
						skipRendering = true
					}
				}
			}

			if !skipRendering {
				fmt.Printf("   Rendering: %s\n", htmlRelPath)
				b.mu.Lock()
				anyPostChanged = true
				b.mu.Unlock()
				b.rnd.RenderPage(destPath, models.PageData{
					Title:        post.Title,
					Description:  post.Description,
					Content:      template.HTML(htmlContent),
					Meta:         metaData,
					BaseURL:      cfg.BaseURL,
					BuildVersion: cfg.BuildVersion,
					TabTitle:     post.Title + " | " + cfg.Title,
					Permalink:    fullLink,
					Image:        imagePath,
					HasMath:      post.HasMath,
					HasMermaid:   post.HasMermaid,
					TOC:          toc,
					Config:       cfg,
				})
			}

			// 7. Shared Data Update
			b.mu.Lock()
			// Ensure the link is correct for the current build (even if from cache)
			post.Link = cfg.BaseURL + "/" + htmlRelPath

			if post.Pinned {
				pinnedPosts = append(pinnedPosts, post)
			} else {
				allPosts = append(allPosts, post)
			}
			for _, t := range post.Tags {
				key := strings.ToLower(strings.TrimSpace(t))
				tagMap[key] = append(tagMap[key], post)
			}
			searchRecord.ID = len(indexedPosts)
			indexedPosts = append(indexedPosts, models.IndexedPost{
				Record:    searchRecord,
				WordFreqs: wordFreqs,
				DocLen:    docLen,
			})
			b.mu.Unlock()

			// Progress tracking
			count := atomic.AddInt32(&processedCount, 1)
			if count%10 == 0 || count == int32(totalFiles) {
				fmt.Printf("   📊 Progress: %d/%d posts processed\n", count, totalFiles)
			}

		}(path)
	}

	wg.Wait()
	// --- PARALLELIZATION END ---

	// --- PARALLEL SOCIAL CARD GENERATION START ---
	if len(socialCardTasks) > 0 {
		fmt.Printf("🖼️  Generating %d social cards in parallel...\n", len(socialCardTasks))
		var cardWg sync.WaitGroup
		cardSem := make(chan struct{}, runtime.NumCPU()) // Use CPU count workers for image processing

		for _, task := range socialCardTasks {
			cardWg.Add(1)
			cardSem <- struct{}{}

			go func(t socialCardTask) {
				defer cardWg.Done()
				defer func() { <-cardSem }()

				fmt.Printf("   🎨 %s\n", t.relPath)
				err := generators.GenerateSocialCard(
					utils.GetString(t.metaData, "title"),
					utils.GetString(t.metaData, "description"),
					utils.GetString(t.metaData, "date"),
					t.cardDestPath,
					faviconPath,
					fontsDir,
				)
				if err != nil {
					fmt.Printf("      ⚠️ Failed to generate card: %v\n", err)
				} else if frontmatterHash, hashErr := utils.GetFrontmatterHash(t.metaData); hashErr == nil {
					b.mu.Lock()
					b.socialCardCache.Hashes[t.path] = frontmatterHash
					b.mu.Unlock()
				}
			}(task)
		}

		cardWg.Wait()
	}
	// --- PARALLEL SOCIAL CARD GENERATION END ---

	// Check if any posts were deleted or if we have a mismatch
	if !anyPostChanged {
		if len(filesToProcess) != len(allPosts)+len(pinnedPosts) {
			anyPostChanged = true
		}
		// If cache has more entries than current files, something was deleted
		if len(b.buildCache.Posts) > len(filesToProcess) {
			anyPostChanged = true
			// Clean up stale cache entries
			activeFiles := make(map[string]bool)
			for _, f := range filesToProcess {
				activeFiles[f] = true
			}
			for path := range b.buildCache.Posts {
				if !activeFiles[path] {
					delete(b.buildCache.Posts, path)
				}
			}
		}
	}

	utils.SortPosts(allPosts)
	utils.SortPosts(pinnedPosts)

	// --- HOME CARD GENERATION START ---
	homeCardPath := "public/static/images/cards/home.webp"
	genHomeCard := false
	if forceSocialRebuild {
		genHomeCard = true
	} else {
		if _, err := os.Stat(homeCardPath); os.IsNotExist(err) {
			genHomeCard = true
		}
	}

	if genHomeCard {
		fmt.Println("   🖼️  Generating Home Social Card...")
		err := generators.GenerateSocialCard(
			cfg.Title,
			cfg.Description,
			"",
			homeCardPath,
			faviconPath,
			fontsDir,
		)
		if err != nil {
			fmt.Printf("      ⚠️ Failed to generate home card: %v\n", err)
		}
	}
	// --- HOME CARD GENERATION END ---

	// --- PAGINATION START ---
	if shouldForce || anyPostChanged {
		postsPerPage := cfg.PostsPerPage
		totalPages := int(math.Ceil(float64(len(allPosts)) / float64(postsPerPage)))

		if totalPages == 0 {
			totalPages = 1
		}

		for i := 1; i <= totalPages; i++ {
			start := (i - 1) * postsPerPage
			end := start + postsPerPage
			if end > len(allPosts) {
				end = len(allPosts)
			}

			pagePosts := allPosts[start:end]

			destPath := "public/index.html"
			permalink := cfg.BaseURL + "/"
			if i > 1 {
				destPath = fmt.Sprintf("public/page/%d/index.html", i)
				permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, i)
				_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			}

			paginator := models.Paginator{
				CurrentPage: i,
				TotalPages:  totalPages,
				HasPrev:     i > 1,
				HasNext:     i < totalPages,
				FirstURL:    cfg.BaseURL + "/#latest",
				LastURL:     fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages),
			}

			if i > 2 {
				paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i-1)
			} else if i == 2 {
				paginator.PrevURL = cfg.BaseURL + "/#latest"
			}

			if i < totalPages {
				paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, i+1)
			}

			// Only show pinned posts on the first page
			var currentPinnedPosts []models.PostMetadata
			if i == 1 {
				currentPinnedPosts = pinnedPosts
			}

			b.rnd.RenderIndex(destPath, models.PageData{
				Title:        cfg.Title,
				Posts:        pagePosts,
				PinnedPosts:  currentPinnedPosts,
				BaseURL:      cfg.BaseURL,
				BuildVersion: cfg.BuildVersion,
				TabTitle:     cfg.Title,
				Description:  cfg.Description,
				Permalink:    permalink,
				Image:        cfg.BaseURL + "/static/images/cards/home.webp",
				Paginator:    paginator,
				Config:       cfg,
			})
		}
	}
	// --- PAGINATION END ---

	if !has404 {
		dest404 := "public/404.html"
		src404 := "templates/404.html"

		shouldBuild404 := false
		if shouldForce {
			shouldBuild404 = true
		} else {
			infoDest, errDest := os.Stat(dest404)
			infoSrc, errSrc := os.Stat(src404)

			if os.IsNotExist(errDest) {
				shouldBuild404 = true
			} else if errSrc == nil && infoSrc.ModTime().After(infoDest.ModTime()) {
				shouldBuild404 = true
			}
		}

		if shouldBuild404 {
			b.rnd.Render404(dest404, models.PageData{
				BaseURL:      cfg.BaseURL,
				BuildVersion: cfg.BuildVersion,
				Config:       cfg,
				TabTitle:     "404 - Page Not Found | " + cfg.Title,
			})
			fmt.Println("📄 404 page rendered.")
		}
	}

	if shouldForce || anyPostChanged {
		var allTags []models.TagData
		for t, posts := range tagMap {
			allTags = append(allTags, models.TagData{
				Name:  t,
				Count: len(posts),
				Link:  fmt.Sprintf("%s/tags/%s.html", cfg.BaseURL, t),
			})
		}
		sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

		b.rnd.RenderPage("public/tags/index.html", models.PageData{
			Title:        "All Tags",
			IsTagsIndex:  true,
			AllTags:      allTags,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Permalink:    cfg.BaseURL + "/tags/index.html",
			Image:        cfg.BaseURL + "/static/images/favicon.webp",
			TabTitle:     "All Topics | " + cfg.Title,
			Config:       cfg,
		})

		for t, posts := range tagMap {
			utils.SortPosts(posts)
			b.rnd.RenderPage(fmt.Sprintf("public/tags/%s.html", t), models.PageData{
				Title:        "#" + t,
				IsIndex:      true,
				Posts:        posts,
				BaseURL:      cfg.BaseURL,
				BuildVersion: cfg.BuildVersion,
				Permalink:    fmt.Sprintf("%s/tags/%s.html", cfg.BaseURL, t),
				Image:        cfg.BaseURL + "/static/images/favicon.webp",
				TabTitle:     "#" + t + " | " + cfg.Title,
				Config:       cfg,
			})
		}
	}

	// Graph rendering logic
	renderGraph := false
	if shouldForce || anyPostChanged {
		renderGraph = true
	} else {
		// Also check if graph template changed
		if info, err := os.Stat("templates/graph.html"); err == nil {
			if destInfo, err := os.Stat("public/graph.html"); err == nil {
				if info.ModTime().After(destInfo.ModTime()) {
					renderGraph = true
				}
			} else {
				renderGraph = true
			}
		}
	}

	if renderGraph {
		b.rnd.RenderGraph("public/graph.html", models.PageData{
			Title:        "Graph View",
			TabTitle:     "Knowledge Graph | " + cfg.Title,
			BaseURL:      cfg.BaseURL,
			BuildVersion: cfg.BuildVersion,
			Config:       cfg,
		})
	}

	if shouldForce || anyPostChanged {
		allContent := append(allPosts, pinnedPosts...)
		generators.GenerateSitemap(cfg.BaseURL, allContent, tagMap)
		generators.GenerateRSS(cfg.BaseURL, allContent, cfg.Title, cfg.Description)
		if err := generators.GenerateSearchIndex("public", indexedPosts); err != nil {
			fmt.Printf("⚠️ Failed to generate search index: %v\n", err)
		}

		graphHash, graphHashErr := utils.GetGraphHash(allContent)
		genGraphJSON := shouldForce
		if !genGraphJSON && graphHashErr == nil {
			if b.socialCardCache.GraphHash != graphHash {
				genGraphJSON = true
			}
		}

		if genGraphJSON {
			generators.GenerateGraph(cfg.BaseURL, allContent)
			if graphHashErr == nil {
				b.socialCardCache.GraphHash = graphHash
			}
			fmt.Println("🕸️  Knowledge Graph regenerated.")
		}
	}

	// --- PWA GENERATION START ---
	if err := generators.GenerateSW("public", cfg.BuildVersion, shouldForce, cfg.BaseURL); err != nil {
		fmt.Printf("⚠️ Failed to generate Service Worker: %v\n", err)
	}

	if err := generators.GenerateManifest("public", cfg.BaseURL, cfg.Title, cfg.Description, shouldForce); err != nil {
		fmt.Printf("⚠️ Failed to generate Web Manifest: %v\n", err)
	}

	if err := generators.GeneratePWAIcons(faviconPath, "public/static/images"); err != nil {
		fmt.Printf("⚠️ Failed to generate PWA icons: %v\n", err)
	}
	// --- PWA GENERATION END ---

	fmt.Println("✅ Build Complete.")
}
