package services

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/search"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// ParsedMarkdownResult holds the output of the markdown parsing phase
type ParsedMarkdownResult struct {
	HTMLContent     string
	MetaData        map[string]interface{}
	Post            models.PostMetadata
	SearchRecord    models.PostRecord
	TOC             []models.TOCEntry
	FrontmatterHash string
	PlainText       string
	SSRHashes       []string
	WordFreqs       map[string]int
	DocLen          int
	StemMap         map[string]string
	PositionalIndex map[string][]int
}

// ParseMarkdown handles the safe parsing and processing of markdown files
func ParseMarkdown(
	ctx context.Context,
	source []byte,
	path string,
	version string,
	cleanHtmlRelPath string,
	htmlRelPath string,
	mdPool *sync.Pool,
	cfg *config.Config,
	nativeRenderer *native.Renderer,
	diagramAdapter *cache.DiagramCacheAdapter,
	mu *sync.Mutex,
) (*ParsedMarkdownResult, error) {

	res := &ParsedMarkdownResult{}

	// Safe markdown parsing with panic recovery
	var docNode ast.Node
	var parseErr error
	var mdCtx parser.Context
	func() {
		defer func() {
			if r := recover(); r != nil {
				parseErr = fmt.Errorf("panic during markdown parsing: %v", r)
			}
		}()

		mdCtx = parser.NewContext()
		mdParser.WithContext(mdCtx, ctx)
		mdCtx.Set(mdParser.ContextKeyFilePath, path)

		mdEngine := mdPool.Get().(goldmark.Markdown)
		defer mdPool.Put(mdEngine)

		docNode = mdEngine.Parser().Parse(text.NewReader(source), parser.WithContext(mdCtx))
	}()

	if parseErr != nil {
		return nil, parseErr
	}

	// Use BufferPool
	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	mdEngineForRender := mdPool.Get().(goldmark.Markdown)
	errRender := mdEngineForRender.Renderer().Render(buf, source, docNode)
	mdPool.Put(mdEngineForRender)

	if errRender != nil {
		return nil, fmt.Errorf("failed to render markdown: %w", errRender)
	}
	res.HTMLContent = buf.String()

	var diagramCache map[string]string
	if diagramAdapter != nil {
		diagramCache = diagramAdapter.AsMap()
	}

	res.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	if bytes.Contains(source, []byte("$")) || bytes.Contains(source, []byte("\\(")) {
		var mathHashes []string
		res.HTMLContent, mathHashes = mdParser.RenderMathForHTML(res.HTMLContent, nativeRenderer, diagramCache, mu)
		res.SSRHashes = append(res.SSRHashes, mathHashes...)
	}

	res.MetaData = meta.Get(mdCtx)
	dateStr := utils.GetString(res.MetaData, "date")
	dateObj, _ := time.Parse("2006-01-02", dateStr)
	isPinned, _ := res.MetaData["pinned"].(bool)
	weight, _ := res.MetaData["weight"].(int)
	if w, ok := res.MetaData["weight"].(float64); ok && weight == 0 {
		weight = int(w)
	}
	wordCount := len(strings.Fields(string(source)))
	res.TOC = mdParser.GetTOC(mdCtx)

	postLink := "/" + strings.TrimPrefix(cleanHtmlRelPath, "/")

	res.Post = models.PostMetadata{
		Title: utils.GetString(res.MetaData, "title"), Link: postLink,
		Description: utils.GetString(res.MetaData, "description"), Tags: utils.GetSlice(res.MetaData, "tags"),
		ReadingTime: int(math.Ceil(float64(wordCount) / wordsPerMinute)), Pinned: isPinned, Weight: weight,
		DateObj: dateObj, Draft: utils.GetBool(res.MetaData, "draft"), Version: version,
	}

	res.PlainText = mdParser.GetPlainText(mdCtx)

	// Pre-compute normalized fields for search
	normalizedTags := make([]string, len(res.Post.Tags))
	for i, t := range res.Post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	res.SearchRecord = models.PostRecord{
		ID:              res.Post.Weight, // Temporary ID
		Title:           res.Post.Title,
		NormalizedTitle: strings.ToLower(res.Post.Title),
		Link:            htmlRelPath,
		Description:     res.Post.Description,
		Tags:            res.Post.Tags,
		NormalizedTags:  normalizedTags,
		Content:         res.PlainText,
		Version:         res.Post.Version,
	}

	// Use analyzer for tokenization with stemming and stop word removal
	var sb strings.Builder
	sb.Grow(len(res.SearchRecord.Title) + len(res.SearchRecord.Description) + len(res.PlainText) + 200)
	sb.WriteString(res.SearchRecord.Title)
	sb.WriteByte(' ')
	sb.WriteString(res.SearchRecord.Description)
	sb.WriteByte(' ')
	for _, t := range res.SearchRecord.Tags {
		sb.WriteString(t)
		sb.WriteByte(' ')
	}
	sb.WriteString(res.PlainText)

	// Analyze with stemming and stop words, and capture mapping and positions
	var freshStemMap map[string]string
	var words []string
	var positions map[string][]int
	words, freshStemMap, positions = search.DefaultAnalyzer.AnalyzeWithPositions(sb.String())
	res.StemMap = freshStemMap
	res.PositionalIndex = positions
	res.DocLen = len(words)
	res.WordFreqs = make(map[string]int)
	for _, w := range words {
		if len(w) >= 2 {
			res.WordFreqs[w]++
		}
	}
	res.FrontmatterHash, _ = utils.GetFrontmatterHash(res.MetaData)

	return res, nil
}
