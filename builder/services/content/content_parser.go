package content

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
	Taxonomies  map[string][]string // Generalized taxonomies from metadata
	IsPinned    bool
	Weight      int
	IsDraft     bool
	IsHidden    bool
}

func extractFrontmatter(metadata map[string]any) parsedFrontmatter {
	dateStr := timeutil.ExtractDateStringFromMap(metadata, "date")
	dateObj, _ := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
	weight, _ := metadata["weight"].(int)
	if w, ok := metadata["weight"].(float64); ok && weight == 0 {
		weight = int(w)
	}

	// Extract all possible taxonomy slices. We'll refine this in buildContentMetadata
	// by checking against config if needed, but for now we pull everything that looks like a slice.
	taxonomies := make(map[string][]string)
	for k := range metadata {
		if slice := timeutil.ExtractSliceFromMap(metadata, k); len(slice) > 0 {
			taxonomies[k] = slice
		}
	}

	return parsedFrontmatter{
		Title:       timeutil.ExtractStringFromMap(metadata, "title"),
		Description: timeutil.ExtractStringFromMap(metadata, "description"),
		DateObj:     dateObj,
		Taxonomies:  taxonomies,
		IsPinned:    timeutil.ExtractBoolFromMap(metadata, "pinned"),
		Weight:      weight,
		IsDraft:     timeutil.ExtractBoolFromMap(metadata, "draft"),
		IsHidden:    timeutil.ExtractBoolFromMap(metadata, "hidden"),
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
	Item            models.ContentMetadata
	SearchRecord    models.ContentRecord
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
	D2Expressions   []models.D2Expression
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
	CleanHTMLRelPath     string
	HTMLRelPath          string
	KnownFrontmatterHash string
	KnownReadingTime     int
	BodyOffset           int
	// PreParsedMeta holds YAML frontmatter values decoded by the scanner.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	PreParsedMeta map[string]any
}

// ParseMarkdownMetadata handles the semantic parsing and metadata extraction.
// This is a refactored version using helper functions for better maintainability.
func ParseMarkdownMetadata(ctx context.Context, options ParseOptions) (*ParsedMarkdownResult, error) {
	result := &ParsedMarkdownResult{}

	// Step 1: Strip frontmatter
	result.BodyOnly = stripFrontmatter(options.Source, options.BodyOffset)

	// Step 2: Parse markdown with panic recovery
	docNode, mdCtx, parseErr := parseMarkdownWithRecovery(ctx, result.BodyOnly, options.Path, options.MdPool)
	if parseErr != nil {
		return nil, parseErr
	}

	result.AST = docNode
	result.Context = mdCtx
	result.SSRHashes = mdParser.GetSSRHashes(mdCtx)

	// Step 3: Extract metadata
	result.Metadata = extractMetadata(mdCtx, options.Source, options.PreParsedMeta)

	// Step 4: Extract frontmatter data and build post metadata
	frontmatter := extractFrontmatter(result.Metadata)
	result.TOC = mdParser.GetTOC(mdCtx)

	postLink := navigation.BuildAbsoluteURL(options.Cfg.BaseURL, options.CleanHTMLRelPath)
	readingTime := computeReadingTime(options.Source, options.KnownReadingTime)

	result.Item = buildContentMetadata(frontmatter, postLink, readingTime, options.RelPath)

	// Step 5: Get plain text and build search record
	result.PlainText = mdParser.GetPlainText(mdCtx)
	result.SearchRecord = buildSearchRecord(result.Item, result.PlainText)

	// Step 5.5: Update reading time based on actual plain text (more accurate)
	if options.KnownReadingTime <= 0 && result.PlainText != "" {
		result.Item.ReadingTime = computeReadingTimeFromText(result.PlainText)
	}

	// Step 6: Search Analysis (DEFERRED to background worker)

	// Step 7: Compute frontmatter hash
	taxonomyKeys := make([]string, 0, len(options.Cfg.Taxonomies))
	for k := range options.Cfg.Taxonomies {
		taxonomyKeys = append(taxonomyKeys, k)
	}
	result.FrontmatterHash = computeFrontmatterHash(result.Metadata, options.KnownFrontmatterHash, taxonomyKeys)

	return result, nil
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
func RenderParsedMarkdown(options MarkdownRenderOptions) error {
	source := options.Source
	result := options.Result
	mdPool := options.MdPool
	// nativeRenderer and diagramAdapter are currently unused in the body,
	// but kept for future compatibility or because they were in the signature.

	if result.AST == nil || result.Context == nil {
		return errMissingParsedMarkdownContext
	}

	body := result.BodyOnly
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

	if err := mdEngine.Renderer().Render(buf, body, result.AST); err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}
	result.HTMLContent = buf.String()
	result.HasImages = bytes.Contains(body, []byte("![")) || bytes.Contains(body, []byte("<img"))

	// Math discovery (deferred to global orchestration)
	// Check for raw LaTeX delimiters OR Kosh placeholders (from shortcode preprocessing)
	if bytes.Contains(source, []byte("$")) || bytes.Contains(source, []byte("\\(")) || bytes.Contains(source, []byte("\\[")) || bytes.Contains(source, []byte("<!--KOSH_MATH:")) {
		result.MathExpressions = mdParser.GetMathExpressions(result.Context)
		for _, expr := range result.MathExpressions {
			result.SSRHashes = append(result.SSRHashes, expr.Hash)
		}
	}

	// D2 discovery (deferred to global orchestration)
	// Check for raw D2 blocks OR Kosh placeholders (from shortcode preprocessing)
	if bytes.Contains(source, []byte("```d2")) || bytes.Contains(source, []byte("<!--KOSH_D2:")) {
		result.D2Expressions = mdParser.GetD2Expressions(result.Context)
		for _, expr := range result.D2Expressions {
			result.SSRHashes = append(result.SSRHashes, expr.Hash)
		}
	}

	return nil
}

// ParseMarkdown handles the safe parsing and processing of markdown files
func ParseMarkdown(ctx context.Context, options ParseOptions) (*ParsedMarkdownResult, error) {
	result, err := ParseMarkdownMetadata(ctx, options)
	if err != nil {
		return nil, err
	}

	if err := RenderParsedMarkdown(MarkdownRenderOptions{
		Source:         options.Source,
		Result:         result,
		MdPool:         options.MdPool,
		NativeRenderer: options.NativeRenderer,
		DiagramAdapter: options.DiagramAdapter,
	}); err != nil {
		return nil, err
	}

	// Nil out heavy objects to reduce peak RSS now that HTML is rendered
	result.AST = nil
	result.Context = nil

	return result, nil
}
