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

type parsedFrontmatter struct {
	Title       string
	Description string
	DateObj     time.Time
	Tags        []string
	Pinned      bool
	Weight      int
	Draft       bool
}

func extractFrontmatter(metaData map[string]interface{}) parsedFrontmatter {
	dateStr := utils.GetString(metaData, "date")
	dateObj, _ := time.Parse("2006-01-02", dateStr)
	weight, _ := metaData["weight"].(int)
	if w, ok := metaData["weight"].(float64); ok && weight == 0 {
		weight = int(w)
	}
	return parsedFrontmatter{
		Title:       utils.GetString(metaData, "title"),
		Description: utils.GetString(metaData, "description"),
		DateObj:     dateObj,
		Tags:        utils.GetSlice(metaData, "tags"),
		Pinned:      utils.GetBool(metaData, "pinned"),
		Weight:      weight,
		Draft:       utils.GetBool(metaData, "draft"),
	}
}

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
	ByteOffsets     map[string][]int
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

		// Render with the same engine instance to reduce pool churn/alloc pressure.
		buf := utils.SharedBufferPool.Get()
		defer utils.SharedBufferPool.Put(buf)

		if err := mdEngine.Renderer().Render(buf, source, docNode); err != nil {
			parseErr = fmt.Errorf("failed to render markdown: %w", err)
			return
		}
		res.HTMLContent = buf.String()
	}()

	if parseErr != nil {
		return nil, parseErr
	}

	res.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	if bytes.Contains(source, []byte("$")) || bytes.Contains(source, []byte("\\(")) {
		var cacheLookup func(string) (string, bool)
		if diagramAdapter != nil {
			cacheLookup = diagramAdapter.GetLocal
		}
		var mathHashes []string
		var renderedMath map[string]string
		res.HTMLContent, mathHashes, renderedMath = mdParser.RenderMathForHTML(res.HTMLContent, nativeRenderer, cacheLookup)
		res.SSRHashes = append(res.SSRHashes, mathHashes...)
		if diagramAdapter != nil && len(renderedMath) > 0 {
			diagramAdapter.Merge(renderedMath)
		}
	}

	res.MetaData = meta.Get(mdCtx)
	fm := extractFrontmatter(res.MetaData)
	wordCount := utils.CountWords(source)
	res.TOC = mdParser.GetTOC(mdCtx)

	postLink := utils.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	res.Post = models.PostMetadata{
		Title: fm.Title, Link: postLink,
		Description: fm.Description, Tags: fm.Tags,
		ReadingTime: int(math.Ceil(float64(wordCount) / wordsPerMinute)), Pinned: fm.Pinned, Weight: fm.Weight,
		DateObj: fm.DateObj, Draft: fm.Draft, Version: version,
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

	// Use pooled analyzer for tokenization with stemming and stop word removal
	sb := utils.SharedStringBuilderPool.Get()
	defer utils.SharedStringBuilderPool.Put(sb)

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
	var offsets map[string][]int
	words, freshStemMap, positions, offsets = search.DefaultAnalyzer.AnalyzeWithPositions(sb.String())

	res.WordFreqs = make(map[string]int, len(words)/2)
	for _, w := range words {
		res.WordFreqs[w]++
	}
	res.DocLen = len(words)
	res.StemMap = freshStemMap
	res.PositionalIndex = positions
	res.ByteOffsets = offsets

	res.FrontmatterHash, _ = utils.GetFrontmatterHash(res.MetaData)

	return res, nil
}
