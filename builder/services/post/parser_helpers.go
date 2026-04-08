package post

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
)

const (
	frontmatterDelimiter    = "---"
	frontmatterDelimiterLen = len(frontmatterDelimiter)
	frontmatterParts        = 3
	metadataBuilderExtra    = 200
	wordFreqMapDivisor      = 2
	titleBoost              = 5
	offsetPairSize          = 2
)

// stripFrontmatter extracts the body content, removing frontmatter if present
func stripFrontmatter(source []byte, bodyOffset int) []byte {
	if bodyOffset > 0 && bodyOffset < len(source) {
		return source[bodyOffset:]
	}
	if bytes.HasPrefix(source, []byte(frontmatterDelimiter)) {
		if idx := bytes.Index(source[frontmatterDelimiterLen:], []byte(frontmatterDelimiter)); idx != -1 {
			return source[idx+frontmatterDelimiterLen*2:]
		}
	}
	return source
}

// parseMarkdownWithRecovery parses markdown with panic recovery
func parseMarkdownWithRecovery(
	bodyOnly []byte,
	path string,
	mdPool *sync.Pool,
	ctx context.Context,
) (ast.Node, parser.Context, error) {
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

		mdEngine, ok := mdPool.Get().(goldmark.Markdown)
		if !ok {
			parseErr = fmt.Errorf("mdPool returned unexpected type %T", mdEngine)
			return
		}
		defer mdPool.Put(mdEngine)

		docNode = mdEngine.Parser().Parse(text.NewReader(bodyOnly), parser.WithContext(mdCtx))
	}()

	return docNode, mdCtx, parseErr
}

// extractMetadata extracts metadata from context or frontmatter.
// Expected value types (from YAML decoding): string, bool, int/float64, time.Time, []any, map[string]any.
func extractMetadata(mdCtx parser.Context, source []byte, preParsedMeta map[string]any) map[string]any {
	if preParsedMeta != nil {
		return preParsedMeta
	}

	if bytes.HasPrefix(source, []byte(frontmatterDelimiter)) {
		parts := bytes.SplitN(source, []byte(frontmatterDelimiter), frontmatterParts)
		if len(parts) >= frontmatterParts {
			metadata, _ := hashing.ParseFrontmatter(parts[1])
			if metadata != nil {
				return metadata
			}
		}
	}

	// Return empty map if no metadata found
	return make(map[string]any)
}

// buildPostMetadata creates PostMetadata from frontmatter data
func buildPostMetadata(
	fm parsedFrontmatter,
	postLink string,
	readingTime int,
) models.PostMetadata {
	return models.PostMetadata{
		Title:       fm.Title,
		Link:        postLink,
		Description: fm.Description,
		Tags:        fm.Tags,
		ReadingTime: readingTime,
		Pinned:      fm.Pinned,
		Weight:      fm.Weight,
		DateObj:     fm.DateObj,
		Draft:       fm.Draft,
	}
}

// buildSearchRecord creates a PostRecord for search indexing
func buildSearchRecord(
	post models.PostMetadata,
	htmlRelPath string,
	plainText string,
) models.PostRecord {
	normalizedTags := make([]string, len(post.Tags))
	for i, t := range post.Tags {
		normalizedTags[i] = strings.ToLower(t)
	}

	return models.PostRecord{
		ID:              0, // Temporary ID, assigned in post_service.go
		Title:           post.Title,
		NormalizedTitle: strings.ToLower(post.Title),
		Link:            htmlRelPath,
		Description:     post.Description,
		Tags:            post.Tags,
		NormalizedTags:  normalizedTags,
		Content:         plainText,
		Date:            post.DateObj.Unix(),
	}
}

// tokenizeSearchData performs search tokenization and builds search-related fields
func tokenizeSearchData(
	searchRecord models.PostRecord,
	plainText string,
) (wordFreqs map[string]int, docLen int, stemMap map[string]string, positionalIndex map[string][]uint32, byteOffsets map[string][]uint32) {
	sb := pools.SharedStringBuilderPool.Get()
	defer pools.SharedStringBuilderPool.Put(sb)

	sb.Grow(len(searchRecord.Title) + len(searchRecord.Description) + len(plainText) + metadataBuilderExtra)
	sb.WriteString(searchRecord.Title)
	sb.WriteByte(' ')
	sb.WriteString(searchRecord.Description)
	sb.WriteByte(' ')
	for _, t := range searchRecord.Tags {
		sb.WriteString(t)
		sb.WriteByte(' ')
	}
	metaOffset := sb.Len()
	sb.WriteString(plainText)

	words, freshStemMap, positions, rawOffsets := core.DefaultAnalyzer.AnalyzeWithPositions(sb.String())

	wordFreqs = make(map[string]int, len(words)/wordFreqMapDivisor)
	for _, w := range words {
		wordFreqs[w]++
	}

	// Apply title boost to frequencies
	titleTokens := core.DefaultAnalyzer.Analyze(searchRecord.Title)
	for _, t := range titleTokens {
		wordFreqs[t] += titleBoost
	}

	docLen = len(words)
	stemMap = freshStemMap

	// Encode positions using delta encoding
	positionalIndex = make(map[string][]uint32, len(positions))
	for term, posList := range positions {
		positionalIndex[term] = models.EncodePositions(posList)
	}

	// Shift and encode offsets using delta format
	byteOffsets = make(map[string][]uint32, len(rawOffsets))
	for term, termOffsets := range rawOffsets {
		bodyOffsets := make([]int, 0, len(termOffsets))
		for i := 0; i < len(termOffsets); i += offsetPairSize {
			start := termOffsets[i]
			end := termOffsets[i+1]
			if start >= metaOffset {
				bodyOffsets = append(bodyOffsets, start-metaOffset, end-metaOffset)
			}
		}
		if len(bodyOffsets) > 0 {
			byteOffsets[term] = models.EncodeOffsets(bodyOffsets)
		}
	}

	return wordFreqs, docLen, stemMap, positionalIndex, byteOffsets
}

// computeReadingTime calculates reading time if not already known
func computeReadingTime(source []byte, knownReadingTime int) int {
	if knownReadingTime > 0 {
		return knownReadingTime
	}
	wordCount := timeutil.CountWords(source)
	return int(math.Ceil(float64(wordCount) / wordsPerMinute))
}

// computeFrontmatterHash computes the frontmatter hash if not already known.
// Expected value types (from YAML decoding): string, bool, int/float64, time.Time, []any, map[string]any.
func computeFrontmatterHash(metadata map[string]any, knownHash string) string {
	if knownHash != "" {
		return knownHash
	}
	hash, _ := hashing.GetFrontmatterHash(metadata)
	return hash
}
