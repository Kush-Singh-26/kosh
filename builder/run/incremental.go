package run

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/search"
	"my-ssg/builder/utils"
)

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	tp := filepath.ToSlash(templatePath)
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.TemplateDir)) {
		if strings.HasSuffix(tp, "layout.html") {
			return nil
		}
		return []string{}
	}
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.StaticDir)) {
		return nil
	}

	switch tp {
	case "kosh.yaml":
		return nil
	case "builder/generators/pwa.go":
		return []string{}
	default:
		return nil
	}
}

// BuildChanged rebuilds only the changed file (for watch mode)
func (b *Builder) BuildChanged(changedPath string) {
	if strings.HasSuffix(changedPath, ".md") && strings.HasPrefix(changedPath, "content") {
		fmt.Printf("⚡ Quick rebuild for: %s\n", changedPath)
		b.buildSinglePost(changedPath)
		// Sync VFS changes to disk (differential)
		if err := utils.SyncVFS(b.DestFs, "public", b.rnd.GetRenderedFiles()); err != nil {
			fmt.Printf("❌ Sync failed: %v\n", err)
		}
		b.rnd.ClearRenderedFiles()
		return
	}

	fmt.Printf("⚡ Full rebuild needed for: %s\n", changedPath)
	b.Build()
	b.SaveCaches()
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Builder) buildSinglePost(path string) {
	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		b.Build()
		return
	}

	context := parser.NewContext()
	reader := text.NewReader(source)
	b.md.Parser().Parse(reader, parser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	cacheKey := utils.NormalizeCacheKey(path)
	b.mu.Lock()
	cached, exists := b.buildCache.Posts[cacheKey]
	b.mu.Unlock()

	if exists && cached.FrontmatterHash == newFrontmatterHash {
		fmt.Printf("   📝 Content-only change detected. Fast rebuild...\n")
		b.buildContentOnly(path)
		b.SaveCaches()
	} else {
		if exists {
			fmt.Printf("   🔄 Frontmatter changed. Full rebuild needed...\n")
		}
		b.mu.Lock()
		delete(b.buildCache.Posts, cacheKey)
		b.mu.Unlock()
		b.Build()
		b.SaveCaches()
	}
}

// buildContentOnly rebuilds just a single post's HTML without regenerating global pages
func (b *Builder) buildContentOnly(path string) {
	cfg := b.cfg

	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		fmt.Printf("   ❌ Error reading %s: %v\n", path, err)
		return
	}

	info, _ := b.SourceFs.Stat(path)
	relPath, _ := filepath.Rel("content", path)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	destPath := filepath.Join("public", htmlRelPath)
	fullLink := cfg.BaseURL + "/" + htmlRelPath

	context := parser.NewContext()
	reader := text.NewReader(source)
	docNode := b.md.Parser().Parse(reader, parser.WithContext(context))

	var buf bytes.Buffer
	if err := b.md.Renderer().Render(&buf, source, docNode); err != nil {
		fmt.Printf("   ❌ Error rendering %s: %v\n", path, err)
		return
	}
	htmlContent := buf.String()

	if orderedPairs := mdParser.GetD2SVGPairSlice(context); orderedPairs != nil {
		htmlContent = mdParser.ReplaceD2BlocksWithThemeSupport(htmlContent, orderedPairs)
	}

	hasMath := strings.Contains(string(source), "$") || strings.Contains(string(source), "\\(")
	if hasMath {
		htmlContent = mdParser.RenderMathForHTML(htmlContent, b.native, b.buildCache.DiagramCache, &b.mu)
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
	hasD2 := mdParser.HasD2(context)

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
		HasMermaid:  hasD2,
	}

	searchRecord := models.PostRecord{
		Title:       post.Title,
		Link:        htmlRelPath,
		Description: post.Description,
		Tags:        post.Tags,
		Content:     plainText,
	}

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
		HasMermaid:      hasD2,
	}
	b.mu.Unlock()

	if isDraft && !cfg.IncludeDrafts {
		fmt.Printf("   ⏩ Skipping draft: %s\n", relPath)
		return
	}

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
		HasMermaid:   hasD2,
		TOC:          toc,
		Config:       cfg,
	})

	fmt.Printf("   ✅ Content-only rebuild complete for: %s\n", htmlRelPath)
}
