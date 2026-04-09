package post

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	renderSvc "github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
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

func extractFrontmatter(metadata map[string]any) parsedFrontmatter {
	dateStr := timeutil.ExtractDateStringFromMap(metadata, "date")
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

// ParsedMarkdownResult holds the output of the markdown parsing phase.
type ParsedMarkdownResult struct {
	AST         ast.Node
	Context     parser.Context
	HTMLContent string
	// Metadata contains YAML frontmatter decoded values.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
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

// ParseOptions configures markdown parsing and metadata extraction.
type ParseOptions struct {
	Path                 string
	RelPath              string
	Source               []byte
	Info                 fs.FileInfo
	Scanner              scanner.Scanner
	Renderer             renderSvc.Service
	NativeRenderer       *native.Renderer
	MdPool               *sync.Pool
	DiagramAdapter       *cache.DiagramCacheAdapter
	Metrics              *metrics.BuildMetrics
	Cfg                  *config.Config
	CleanHtmlRelPath     string
	HtmlRelPath          string
	KnownFrontmatterHash string
	KnownReadingTime     int
	BodyOffset           int
	// PreParsedMeta holds YAML frontmatter values decoded by the scanner.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	PreParsedMeta map[string]any
}

// ParseMarkdownMetadata handles the semantic parsing and metadata extraction.
// This is a refactored version using helper functions for better maintainability.
func ParseMarkdownMetadata(opts ParseOptions) (*ParsedMarkdownResult, error) {
	res := &ParsedMarkdownResult{}

	// Step 1: Strip frontmatter
	res.BodyOnly = stripFrontmatter(opts.Source, opts.BodyOffset)

	// Step 2: Parse markdown with panic recovery
	docNode, mdCtx, parseErr := parseMarkdownWithRecovery(res.BodyOnly, opts.Path, opts.MdPool, context.Background())
	if parseErr != nil {
		return nil, parseErr
	}

	res.AST = docNode
	res.Context = mdCtx
	res.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	// Step 3: Extract metadata
	res.Metadata = extractMetadata(mdCtx, opts.Source, opts.PreParsedMeta)

	// Step 4: Extract frontmatter data and build post metadata
	fm := extractFrontmatter(res.Metadata)
	res.TOC = mdParser.GetTOC(mdCtx)

	postLink := navigation.BuildAbsoluteURL(opts.Cfg.BaseURL, opts.CleanHtmlRelPath)
	readingTime := computeReadingTime(opts.Source, opts.KnownReadingTime)

	res.Post = buildPostMetadata(fm, postLink, readingTime)

	// Step 5: Get plain text and build search record
	res.PlainText = mdParser.GetPlainText(mdCtx)
	res.SearchRecord = buildSearchRecord(res.Post, opts.HtmlRelPath, res.PlainText)

	// Step 6: Search Analysis (DEFERRED to background worker)

	// Step 7: Compute frontmatter hash
	res.FrontmatterHash = computeFrontmatterHash(res.Metadata, opts.KnownFrontmatterHash)

	return res, nil
}

// MarkdownRenderOptions configures HTML rendering for parsed markdown.
type MarkdownRenderOptions struct {
	Source         []byte
	Result         *ParsedMarkdownResult
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
	DiagramAdapter *cache.DiagramCacheAdapter
}

var errMissingParsedMarkdownContext = errors.New("missing AST or Context in ParsedMarkdownResult")

// RenderParsedMarkdown converts the AST to HTML and performs Math discovery
func RenderParsedMarkdown(opts MarkdownRenderOptions) error {
	source := opts.Source
	res := opts.Result
	mdPool := opts.MdPool
	// nativeRenderer and diagramAdapter are currently unused in the body,
	// but kept for future compatibility or because they were in the signature.

	if res.AST == nil || res.Context == nil {
		return errMissingParsedMarkdownContext
	}

	body := res.BodyOnly
	if len(body) == 0 {
		body = source
	}

	mdEngine, ok := mdPool.Get().(goldmark.Markdown)
	if !ok {
		return fmt.Errorf("mdPool returned unexpected type %T", mdEngine)
	}
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
func ParseMarkdown(opts ParseOptions) (*ParsedMarkdownResult, error) {
	res, err := ParseMarkdownMetadata(opts)
	if err != nil {
		return nil, err
	}

	if err := RenderParsedMarkdown(MarkdownRenderOptions{
		Source:         opts.Source,
		Result:         res,
		MdPool:         opts.MdPool,
		NativeRenderer: opts.NativeRenderer,
		DiagramAdapter: opts.DiagramAdapter,
	}); err != nil {
		return nil, err
	}

	// Nil out heavy objects to reduce peak RSS now that HTML is rendered
	res.AST = nil
	res.Context = nil

	return res, nil
}
