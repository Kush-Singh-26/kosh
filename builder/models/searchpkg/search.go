package searchpkg
 
//go:generate msgp

import (
	"slices"
	"sort"
)

// SearchIngestor defines an interface for pipelined search indexing.
type SearchIngestor interface {
	Add(item IndexedContent)
}

// ContentRecord represents a search-optimized record for BM25 indexing and
// search functionality. It contains normalized fields for efficient text
// matching and version-aware search.
type ContentRecord struct {
	Title       string
	Link        string
	Description string
	Taxonomies  map[string][]string // All taxonomies (e.g., tags, categories)
	NormalizedTaxs  map[string][]string // Lowercase taxonomies for search
	Content         string              // Raw plain text for snippet extraction
	Date            int64               // Unix timestamp for recency scoring
}

// IndexedContent bundles a search record with pre-computed word frequencies for BM25
type IndexedContent struct {
	DenseID         uint32            // monotonic 0-based index
	Record          ContentRecord
	SourcePath      string `msgp:"-"`
	WordFreqs       map[string]int
	DocLen          int
	StemMap         map[string]string // original word -> stem
	PositionalIndex map[string][]uint32
}

// CurrentSchemaVersion is the active search schema version.
const CurrentSchemaVersion = 25

// SearchIndex v2 — Flat Lexicon + CSR Posting Table
type SearchIndex struct {
	SchemaVersion int64

	// Metadata store: dense uint32 -> ContentRecord
	Items    []ContentRecord // indexed by dense doc ID (uint32)
	ItemLens []int32         // parallel: word count per doc

	// Statistics
	AvgDocLen  float64
	TotalItems int64
	Ranking    SearchRankingConfig // Configurable ranking weights

	// StemMap: stemmed -> original forms (kept, small & useful)
	StemMap map[string][]string

	// Flat sorted lexicon (dictionary)
	Terms []string

	// CSR Posting Table
	// For term at index i:
	//   - document IDs: DocIDs[ PostingOffsets[i] : PostingOffsets[i+1] ]
	//   - positions:    Positions[ PosOffsets[i] : PosOffsets[i+1] ]
	PostingOffsets []uint32 // len = len(Terms)+1
	DocIDs         []uint32 // flat list of delta-encoded doc IDs
	
	// DocPosOffsets[k] is the index into Positions for the start of doc k's positions.
	// k is the index into DocIDs.
	// DocPosOffsets[k+1] - DocPosOffsets[k] is the frequency for doc k.
	DocPosOffsets  []uint32 // parallel to DocIDs (plus one sentinel at end of each term block?)
	// Actually, easier to make it just DocPosOffsets[ PostingOffsets[i] : PostingOffsets[i+1]+1 ]
	
	PosOffsets     []uint32 // len = len(Terms)+1 (into Positions)
	Positions      []uint32 // flat list of delta-encoded positions

	// Title-only inverted list for fast title scoring
	// Parallel to Terms: TitlePostings[i] = doc IDs that contain term i in title
	TitlePostingOffsets []uint32
	TitleDocIDs         []uint32
}

// LookupTerm finds a term's index in Terms using binary search. Returns -1 if not found.
func (idx *SearchIndex) LookupTerm(term string) int {
	i := slices.Index(idx.Terms, term)
	if i >= 0 {
		return i
	}
	// Fallback to binary search if slices.Index is not optimized or for clarity
	n := len(idx.Terms)
	j := sort.Search(n, func(j int) bool {
		return idx.Terms[j] >= term
	})
	if j < n && idx.Terms[j] == term {
		return j
	}
	return -1
}

// GetPostings returns the (docIDs_slice, positions_slice) for a term index.
// Both slices are views into the CSR tables, zero-allocation.
func (idx *SearchIndex) GetPostings(termIdx int) (docIDs []uint32, positions []uint32) {
	if termIdx < 0 || termIdx >= len(idx.Terms) {
		return nil, nil
	}
	docIDs = idx.DocIDs[idx.PostingOffsets[termIdx]:idx.PostingOffsets[termIdx+1]]
	positions = idx.Positions[idx.PosOffsets[termIdx]:idx.PosOffsets[termIdx+1]]
	return docIDs, positions
}

// GetTitlePostings returns doc IDs containing the term in their title.
func (idx *SearchIndex) GetTitlePostings(termIdx int) []uint32 {
	if termIdx < 0 || termIdx >= len(idx.Terms) || len(idx.TitlePostingOffsets) <= termIdx {
		return nil
	}
	return idx.TitleDocIDs[idx.TitlePostingOffsets[termIdx]:idx.TitlePostingOffsets[termIdx+1]]
}

// DecodeDocIDs decodes a delta-encoded doc ID list into absolute IDs.
func DecodeDocIDs(deltas []uint32) []uint32 {
	if len(deltas) == 0 {
		return nil
	}
	ids := make([]uint32, len(deltas))
	ids[0] = deltas[0]
	for i := 1; i < len(deltas); i++ {
		ids[i] = ids[i-1] + deltas[i]
	}
	return ids
}

// DecodePositions decodes delta-encoded positions into absolute positions
func DecodePositions(deltas []uint32) []int {
	if len(deltas) == 0 {
		return nil
	}
	positions := make([]int, len(deltas))
	positions[0] = int(deltas[0])
	for i := 1; i < len(deltas); i++ {
		positions[i] = positions[i-1] + int(deltas[i])
	}
	return positions
}

// DecodeOffsets decodes delta-encoded byte offsets into [start, end, start, end, ...]
func DecodeOffsets(deltas []uint32) []int {
	if len(deltas) == 0 {
		return nil
	}
	n := len(deltas) / 2
	result := make([]int, 0, n*2)
	absStart := int(deltas[0])
	for i := 0; i < n; i++ {
		length := int(deltas[i*2+1])
		result = append(result, absStart, absStart+length)
		if i+1 < n {
			absStart += int(deltas[i*2+2])
		}
	}
	return result
}

// EncodePositions encodes absolute positions into delta format
func EncodePositions(positions []int) []uint32 {
	if len(positions) == 0 {
		return nil
	}
	deltas := make([]uint32, len(positions))
	deltas[0] = uint32(positions[0])
	for i := 1; i < len(positions); i++ {
		deltas[i] = uint32(positions[i] - positions[i-1])
	}
	return deltas
}

// EncodeOffsets encodes [start, end, start, end, ...] into delta format
func EncodeOffsets(pairs []int) []uint32 {
	if len(pairs) == 0 {
		return nil
	}
	deltas := make([]uint32, 0, len(pairs))
	for i := 0; i < len(pairs); i += 2 {
		start := pairs[i]
		end := pairs[i+1]
		length := end - start
		if i == 0 {
			deltas = append(deltas, uint32(start), uint32(length))
		} else {
			deltas = append(deltas, uint32(start-pairs[i-2]), uint32(length))
		}
	}
	return deltas
}
