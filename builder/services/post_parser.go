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

type ParseOptions struct {
	Source []byte
	Path   string

	Version          string
	CleanHtmlRelPath string
	HtmlRelPath      string

	MdPool         *sync.Pool
	Cfg            *config.Config
	NativeRenderer *native.Renderer
	DiagramAdapter *cache.DiagramCacheAdapter
	Mu             *sync.Mutex

	KnownFrontmatterHash string
	KnownReadingTime     int
	BodyOffset           int
	PreParsedMeta        map[string]any
}

type parsedFrontmatter struct {
	Title       string
	Description string
	DateObj     time.Time
	Tags        []string
	Pinned      bool
	Weight      int
	Draft       bool
}

func extractFrontmatter(metaData map[string]any) parsedFrontmatter {
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
	AST             ast.Node
	Context         parser.Context
	HTMLContent     string
	MetaData        map[string]any
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
	MathExpressions []native.MathExpression
	HasImages       bool
	BodyOnly        []byte
}

// ParseMarkdownMetadata handles the semantic parsing and metadata extraction
func ParseMarkdownMetadata(
	ctx context.Context,
	source []byte,
	path string,
	version string,
	cleanHtmlRelPath string,
	htmlRelPath string,
	mdPool *sync.Pool,
	cfg *config.Config,
	knownFrontmatterHash string,
	knownReadingTime int,
	bodyOffset int,
	preParsedMeta map[string]any,
) (*ParsedMarkdownResult, error) {
	res := &ParsedMarkdownResult{}

	// Strip frontmatter to avoid double parse
	bodyOnly := source
	if bodyOffset > 0 && bodyOffset < len(source) {
		bodyOnly = source[bodyOffset:]
	} else if bytes.HasPrefix(source, []byte("---")) {
		if idx := bytes.Index(source[3:], []byte("---")); idx != -1 {
			bodyOnly = source[idx+6:]
		}
	}
	res.BodyOnly = bodyOnly

	var docNode ast.Node
	var mdCtx parser.Context
	var parseErr error

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

		docNode = mdEngine.Parser().Parse(text.NewReader(bodyOnly), parser.WithContext(mdCtx))
	}()

	if parseErr != nil {
		return nil, parseErr
	}

	res.AST = docNode
	res.Context = mdCtx
	res.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	if preParsedMeta != nil {
		res.MetaData = preParsedMeta
	} else if bytes.HasPrefix(source, []byte("---")) {
		parts := bytes.SplitN(source, []byte("---"), 3)
		if len(parts) >= 3 {
			res.MetaData, _ = utils.ParseFrontmatter(parts[1])
		}
	}

	if res.MetaData == nil {
		res.MetaData = meta.Get(mdCtx)
	}

	fm := extractFrontmatter(res.MetaData)
	res.TOC = mdParser.GetTOC(mdCtx)

	postLink := utils.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	readingTime := knownReadingTime
	if readingTime <= 0 {
		wordCount := utils.CountWords(source)
		readingTime = int(math.Ceil(float64(wordCount) / wordsPerMinute))
	}

	res.Post = models.PostMetadata{
		Title: fm.Title, Link: postLink,
		Description: fm.Description, Tags: fm.Tags,
		ReadingTime: readingTime, Pinned: fm.Pinned, Weight: fm.Weight,
		DateObj: fm.DateObj, Draft: fm.Draft, Version: version,
	}

	res.PlainText = mdParser.GetPlainText(mdCtx)

	// Pre-compute normalized fields for search
	normalizedTags := make([]string, len(res.Post.Tags))
	for i, t := range res.Post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	res.SearchRecord = models.PostRecord{
		ID:              0, // Temporary ID, assigned in post_service.go
		Title:           res.Post.Title,
		NormalizedTitle: strings.ToLower(res.Post.Title),
		Link:            htmlRelPath,
		Description:     res.Post.Description,
		Tags:            res.Post.Tags,
		NormalizedTags:  normalizedTags,
		Content:         res.PlainText,
		Version:         res.Post.Version,
	}

	// Use pooled analyzer for tokenization if search is enabled
	if cfg.Features.Generators.Search {
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
		metaOffset := sb.Len()
		sb.WriteString(res.PlainText)

		words, freshStemMap, positions, rawOffsets := search.DefaultAnalyzer.AnalyzeWithPositions(sb.String())

		res.WordFreqs = make(map[string]int, len(words)/2)
		for _, w := range words {
			res.WordFreqs[w]++
		}
		res.DocLen = len(words)
		res.StemMap = freshStemMap
		res.PositionalIndex = positions

		// Shift offsets to be relative to PlainText content only
		res.ByteOffsets = make(map[string][]int, len(rawOffsets))
		for term, termOffsets := range rawOffsets {
			bodyOffsets := make([]int, 0, len(termOffsets))
			for i := 0; i < len(termOffsets); i += 2 {
				start := termOffsets[i]
				end := termOffsets[i+1]
				if start >= metaOffset {
					bodyOffsets = append(bodyOffsets, start-metaOffset, end-metaOffset)
				}
			}
			if len(bodyOffsets) > 0 {
				res.ByteOffsets[term] = bodyOffsets
			}
		}
	}

	if knownFrontmatterHash != "" {
		res.FrontmatterHash = knownFrontmatterHash
	} else {
		res.FrontmatterHash, _ = utils.GetFrontmatterHash(res.MetaData)
	}

	return res, nil
}

// RenderParsedMarkdown converts the AST to HTML and performs Math discovery
func RenderParsedMarkdown(
	source []byte,
	res *ParsedMarkdownResult,
	mdPool *sync.Pool,
	nativeRenderer *native.Renderer,
	diagramAdapter *cache.DiagramCacheAdapter,
) error {
	if res.AST == nil || res.Context == nil {
		return fmt.Errorf("missing AST or Context in ParsedMarkdownResult")
	}

	body := res.BodyOnly
	if len(body) == 0 {
		body = source
	}

	mdEngine := mdPool.Get().(goldmark.Markdown)
	defer mdPool.Put(mdEngine)

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	if err := mdEngine.Renderer().Render(buf, body, res.AST); err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}
	res.HTMLContent = buf.String()
	res.HasImages = bytes.Contains(body, []byte("![")) || bytes.Contains(body, []byte("<img"))

	// Math discovery (deferred to global orchestration)
	if bytes.Contains(source, []byte("$")) || bytes.Contains(source, []byte("\\(")) || bytes.Contains(source, []byte("\\[")) {
		res.MathExpressions = mdParser.GetMathExpressions(res.Context)
		for _, expr := range res.MathExpressions {
			res.SSRHashes = append(res.SSRHashes, expr.Hash)
		}
	}

	return nil
}

// ParseMarkdown handles the safe parsing and processing of markdown files (Legacy/Convenience)
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
	knownFrontmatterHash string,
	knownReadingTime int,
	bodyOffset int,
	preParsedMeta map[string]any,
) (*ParsedMarkdownResult, error) {
	res, err := ParseMarkdownMetadata(ctx, source, path, version, cleanHtmlRelPath, htmlRelPath, mdPool, cfg, knownFrontmatterHash, knownReadingTime, bodyOffset, preParsedMeta)
	if err != nil {
		return nil, err
	}

	if err := RenderParsedMarkdown(source, res, mdPool, nativeRenderer, diagramAdapter); err != nil {
		return nil, err
	}

	// Nil out heavy objects to reduce peak RSS now that HTML is rendered
	res.AST = nil
	res.Context = nil

	return res, nil
}
