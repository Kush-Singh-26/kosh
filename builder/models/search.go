package models

// PostRecord represents a search-optimized record for BM25 indexing and
// search functionality. It contains normalized fields for efficient text
// matching and version-aware search.
type PostRecord struct {
	// ID is a uint64 representation of the post's link, used for compact
	// in-memory indexing. Note that search.bin uses decimal strings of this ID
	// for serialization compatibility, while BoltDB cache uses 128-bit hex strings.
	ID              uint64
	Title           string
	NormalizedTitle string // Lowercase title for search
	Link            string
	Description     string
	Tags            []string
	NormalizedTags  []string // Lowercase tags for search
	Content         string   // Raw plain text for snippet extraction
	Date            int64    // Unix timestamp for recency scoring
}

// IndexedPost bundles a search record with pre-computed word frequencies for BM25
type IndexedPost struct {
	Record          PostRecord
	SourcePath      string `msgp:"-"`
	WordFreqs       map[string]int
	DocLen          int
	StemMap         map[string]string // original word -> stem
	PositionalIndex map[string][]uint32
	ByteOffsets     map[string][]uint32
}

// CurrentSchemaVersion is the active search schema version.
const CurrentSchemaVersion = 14

// SearchIndex stores the serialized search index.
type SearchIndex struct {
	SchemaVersion int64
	Posts         map[string]PostRecord
	DocLens       map[string]int64 // postID (string) -> word count
	AvgDocLen     float64
	TotalDocs     int64
	StemMap       map[string][]string // stemmed -> original forms
	NgramIndex    map[string][]string // trigram -> terms (for fuzzy search)

	// Inverted Index: word -> postID (string) -> delta-encoded positions
	// Deltas: [pos1, pos2-pos1, pos3-pos2, ...] (v11+)
	Inverted map[string]map[string][]uint32

	// Byte offsets map: word -> postID (string) -> delta-encoded offsets
	// Format: [start1, length1, start2-start1, length2, ...] (v11+)
	Offsets map[string]map[string][]uint32
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
