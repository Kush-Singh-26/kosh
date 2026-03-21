package services

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

type ParseConfig struct {
	Source               []byte
	Path                 string
	Version              string
	CleanHtmlRelPath     string
	HtmlRelPath          string
	KnownFrontmatterHash string
	KnownReadingTime     int
	BodyOffset           int
	PreParsedMeta        map[string]any
}

type ParseContext struct {
	MdPool         *sync.Pool
	Cfg            *config.Config
	NativeRenderer *native.Renderer
	DiagramAdapter *cache.DiagramCacheAdapter
	MathBatchSize  int
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

func extractFrontmatter(metadata map[string]any) parsedFrontmatter {
	dateStr := timeutil.ExtractStringFromMap(metadata, "date")
	dateObj, _ := time.Parse("2006-01-02", dateStr)
	weight, _ := metadata["weight"].(int)
	if w, ok := metadata["weight"].(float64); ok && weight == 0 {
		weight = int(w)
	}
	return parsedFrontmatter{
		Title:       timeutil.ExtractStringFromMap(metadata, "title"),
		Description: timeutil.ExtractStringFromMap(metadata, "description"),
		DateObj:     dateObj,
		Tags:        timeutil.ExtractSliceFromMap(metadata, "tags"),
		Pinned:      timeutil.ExtractBoolFromMap(metadata, "pinned"),
		Weight:      weight,
		Draft:       timeutil.ExtractBoolFromMap(metadata, "draft"),
	}
}

// ParsedMarkdownResult holds the output of the markdown parsing phase
type ParsedMarkdownResult struct {
	AST             ast.Node
	Context         parser.Context
	HTMLContent     string
	Metadata        map[string]any
	Post            models.PostMetadata
	SearchRecord    models.PostRecord
	TOC             []models.TOCEntry
	FrontmatterHash string
	PlainText       string
	SSRHashes       []string
	WordFreqs       map[string]int
	DocLen          int
	StemMap         map[string]string
	PositionalIndex map[string][]uint32
	ByteOffsets     map[string][]uint32
	MathExpressions []models.MathExpression
	HasImages       bool
	BodyOnly        []byte
}

// ParseMarkdownMetadata handles the semantic parsing and metadata extraction.
// This is a refactored version using helper functions for better maintainability.
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

	// Step 1: Strip frontmatter
	res.BodyOnly = stripFrontmatter(source, bodyOffset)

	// Step 2: Parse markdown with panic recovery
	docNode, mdCtx, parseErr := parseMarkdownWithRecovery(res.BodyOnly, path, mdPool, ctx)
	if parseErr != nil {
		return nil, parseErr
	}

	res.AST = docNode
	res.Context = mdCtx
	res.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	// Step 3: Extract metadata
	res.Metadata = extractMetadata(mdCtx, source, preParsedMeta)

	// Step 4: Extract frontmatter data and build post metadata
	fm := extractFrontmatter(res.Metadata)
	res.TOC = mdParser.GetTOC(mdCtx)

	postLink := navigation.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)
	readingTime := computeReadingTime(source, knownReadingTime)

	res.Post = buildPostMetadata(fm, postLink, readingTime, version)

	// Step 5: Get plain text and build search record
	res.PlainText = mdParser.GetPlainText(mdCtx)
	res.SearchRecord = buildSearchRecord(res.Post, htmlRelPath, res.PlainText)

	// Step 6: Search Analysis (DEFERRED to background worker)

	// Step 7: Compute frontmatter hash
	res.FrontmatterHash = computeFrontmatterHash(res.Metadata, knownFrontmatterHash)

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

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

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

// ParseMarkdown handles the safe parsing and processing of markdown files
func ParseMarkdown(cfg ParseConfig, ctx ParseContext) (*ParsedMarkdownResult, error) {
	res, err := ParseMarkdownMetadata(
		context.Background(),
		cfg.Source,
		cfg.Path,
		cfg.Version,
		cfg.CleanHtmlRelPath,
		cfg.HtmlRelPath,
		ctx.MdPool,
		ctx.Cfg,
		cfg.KnownFrontmatterHash,
		cfg.KnownReadingTime,
		cfg.BodyOffset,
		cfg.PreParsedMeta,
	)
	if err != nil {
		return nil, err
	}

	if err := RenderParsedMarkdown(cfg.Source, res, ctx.MdPool, ctx.NativeRenderer, ctx.DiagramAdapter); err != nil {
		return nil, err
	}

	// Nil out heavy objects to reduce peak RSS now that HTML is rendered
	res.AST = nil
	res.Context = nil

	return res, nil
}
