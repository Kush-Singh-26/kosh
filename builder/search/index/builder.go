package index

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)


const (
	maxIndexWorkers             = 8
	minWorkerCap                = 64
	perWordMapCap               = 4
	minStemMapCap               = 100
)

// tempPostings helps group postings by term before flattening into CSR
type tempPostings struct {
	docIDs    []uint32
	positions [][]uint32 // list of position slices for each docID
}

type partialResult struct {
	items         map[uint32]searchpkg.ContentRecord
	inverted      map[string]*tempPostings
	titleInverted map[string][]uint32
	itemLens     map[uint32]int32
	stemMap      map[string]map[string]bool
	totalLen     int64
	totalDocs    int
	maxDenseID   uint32
}

// Build constructs an in-memory search index from a list of indexed posts.
func Build(indexedPosts []searchpkg.IndexedContent) *searchpkg.SearchIndex {
	sb := NewStreamBuilder(context.Background(), len(indexedPosts))
	for _, ip := range indexedPosts {
		sb.Add(ip)
	}
	return sb.Complete()
}

// StreamBuilder manages concurrent search index building from a stream of posts.
type StreamBuilder struct {
	postChan   chan searchpkg.IndexedContent
	results    []partialResult
	numWorkers int
	wg         sync.WaitGroup
	expectedDocs int
}

// NewStreamBuilder initializes a new pipelined search index builder.
func NewStreamBuilder(ctx context.Context, expectedDocs int) *StreamBuilder {
	numWorkers := min(runtime.NumCPU(), maxIndexWorkers)
	sb := &StreamBuilder{
		postChan:     make(chan searchpkg.IndexedContent, max(expectedDocs, 32)),
		results:      make([]partialResult, numWorkers),
		numWorkers:   numWorkers,
		expectedDocs: expectedDocs,
	}

	for i := 0; i < numWorkers; i++ {
		sb.wg.Add(1)
		workerID := i
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "search index stream build",
			Fn: func() error {
				sb.runWorker(ctx, workerID)
				return nil
			},
			Cleanup: sb.wg.Done,
		})
	}

	return sb
}

func (sb *StreamBuilder) runWorker(ctx context.Context, workerID int) {
	workerCap := max(sb.expectedDocs/sb.numWorkers, minWorkerCap)
	res := partialResult{
		items:         make(map[uint32]searchpkg.ContentRecord, workerCap),
		inverted:      make(map[string]*tempPostings, workerCap*10),
		titleInverted: make(map[string][]uint32, workerCap*2),
		itemLens:      make(map[uint32]int32, workerCap),
		stemMap:       make(map[string]map[string]bool, workerCap),
	}

	for {
		select {
		case ip, ok := <-sb.postChan:
			if !ok {
				sb.results[workerID] = res
				return
			}
			res.items[ip.DenseID] = ip.Record
			res.itemLens[ip.DenseID] = int32(ip.DocLen)
			res.totalLen += int64(ip.DocLen)
			res.totalDocs++
			if ip.DenseID > res.maxDenseID {
				res.maxDenseID = ip.DenseID
			}

			// Index title tokens separately
			titleTokens := core.DefaultAnalyzer.Analyze(strings.ToLower(ip.Record.Title))
			for _, word := range titleTokens {
				res.titleInverted[word] = append(res.titleInverted[word], ip.DenseID)
			}

			// Index body tokens
			for word, positions := range ip.PositionalIndex {
				tp, ok := res.inverted[word]
				if !ok {
					tp = &tempPostings{
						docIDs:    make([]uint32, 0, perWordMapCap),
						positions: make([][]uint32, 0, perWordMapCap),
					}
					res.inverted[word] = tp
				}
				tp.docIDs = append(tp.docIDs, ip.DenseID)
				tp.positions = append(tp.positions, positions)
			}

			// Stem mappings
			for orig, stem := range ip.StemMap {
				oMap, ok := res.stemMap[stem]
				if !ok {
					oMap = make(map[string]bool, 2)
					res.stemMap[stem] = oMap
				}
				oMap[orig] = true
			}
		case <-ctx.Done():
			return
		}
	}
}

// Add enqueues a post for background indexing.
func (sb *StreamBuilder) Add(ip searchpkg.IndexedContent) {
	sb.postChan <- ip
}

// Complete closes the input stream and merges the results into a single index.
func (sb *StreamBuilder) Complete() *searchpkg.SearchIndex {
	close(sb.postChan)
	sb.wg.Wait()

	totalDocs, totalLen, maxDenseID := sb.calculateGlobalStats()
	if totalDocs == 0 {
		return &searchpkg.SearchIndex{SchemaVersion: searchpkg.CurrentSchemaVersion}
	}

	allocSize := max(totalDocs, int(maxDenseID)+1)
	index := &searchpkg.SearchIndex{
		SchemaVersion: searchpkg.CurrentSchemaVersion,
		Items:         make([]searchpkg.ContentRecord, allocSize),
		ItemLens:      make([]int32, allocSize),
		TotalItems:    int64(totalDocs),
		AvgDocLen:     float64(totalLen) / float64(totalDocs),
	}

	// 1. Merge items and stem origins
	masterInverted, masterTitleInverted, stemOrigins := sb.mergePartialResults(index)

	// Flatten StemMap
	index.StemMap = sb.flattenStemMap(stemOrigins)

	// 2. Sort terms and build CSR
	terms := sb.extractSortedTerms(masterInverted)
	index.Terms = terms

	// 3. Pre-calculate sizes and fill CSR tables
	sb.buildCSRTables(index, terms, masterInverted, masterTitleInverted)

	return index
}

func (sb *StreamBuilder) calculateGlobalStats() (totalDocs int, totalLen int64, maxDenseID uint32) {
	for _, r := range sb.results {
		totalDocs += r.totalDocs
		totalLen += r.totalLen
		if r.maxDenseID > maxDenseID {
			maxDenseID = r.maxDenseID
		}
	}
	return
}

func (sb *StreamBuilder) mergePartialResults(index *searchpkg.SearchIndex) (map[string]*tempPostings, map[string][]uint32, map[string]map[string]bool) {
	masterInverted := make(map[string]*tempPostings)
	masterTitleInverted := make(map[string][]uint32)
	stemOrigins := make(map[string]map[string]bool)

	for _, r := range sb.results {
		for id, item := range r.items {
			index.Items[id] = item
			index.ItemLens[id] = r.itemLens[id]
		}
		for word, tp := range r.inverted {
			if mTP, ok := masterInverted[word]; !ok {
				masterInverted[word] = tp
			} else {
				mTP.docIDs = append(mTP.docIDs, tp.docIDs...)
				mTP.positions = append(mTP.positions, tp.positions...)
			}
		}
		for word, ids := range r.titleInverted {
			masterTitleInverted[word] = append(masterTitleInverted[word], ids...)
		}
		for stem, origins := range r.stemMap {
			mOrigins, ok := stemOrigins[stem]
			if !ok {
				stemOrigins[stem] = origins
			} else {
				for o := range origins {
					mOrigins[o] = true
				}
			}
		}
	}
	return masterInverted, masterTitleInverted, stemOrigins
}

func (sb *StreamBuilder) flattenStemMap(stemOrigins map[string]map[string]bool) map[string][]string {
	res := make(map[string][]string, len(stemOrigins))
	for stem, oMap := range stemOrigins {
		origins := make([]string, 0, len(oMap))
		for o := range oMap {
			origins = append(origins, o)
		}
		res[stem] = origins
	}
	return res
}

func (sb *StreamBuilder) extractSortedTerms(masterInverted map[string]*tempPostings) []string {
	terms := make([]string, 0, len(masterInverted))
	for t := range masterInverted {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	return terms
}

func (sb *StreamBuilder) buildCSRTables(index *searchpkg.SearchIndex, terms []string, masterInverted map[string]*tempPostings, masterTitleInverted map[string][]uint32) {
	numTerms := len(terms)
	index.PostingOffsets = make([]uint32, numTerms+1)
	index.PosOffsets = make([]uint32, numTerms+1)
	index.TitlePostingOffsets = make([]uint32, numTerms+1)

	totalPostings, totalPositions, totalTitlePostings := sb.calculateCSRSizes(index, terms, masterInverted, masterTitleInverted)

	index.DocIDs = make([]uint32, totalPostings)
	index.DocPosOffsets = make([]uint32, totalPostings+1)
	index.Positions = make([]uint32, totalPositions)
	index.TitleDocIDs = make([]uint32, totalTitlePostings)

	sb.fillCSRTables(index, terms, masterInverted, masterTitleInverted)
}

func (sb *StreamBuilder) calculateCSRSizes(index *searchpkg.SearchIndex, terms []string, masterInverted map[string]*tempPostings, masterTitleInverted map[string][]uint32) (totalPostings, totalPositions, totalTitlePostings int) {
	for i, term := range terms {
		tp := masterInverted[term]
		sortPostings(tp)
		
		index.PostingOffsets[i] = uint32(totalPostings)
		totalPostings += len(tp.docIDs)

		index.PosOffsets[i] = uint32(totalPositions)
		for _, posList := range tp.positions {
			totalPositions += len(posList)
		}

		if tIDs, ok := masterTitleInverted[term]; ok {
			slices.Sort(tIDs)
			masterTitleInverted[term] = slices.Compact(tIDs)
			index.TitlePostingOffsets[i] = uint32(totalTitlePostings)
			totalTitlePostings += len(masterTitleInverted[term])
		} else {
			index.TitlePostingOffsets[i] = uint32(totalTitlePostings)
		}
	}
	index.PostingOffsets[len(terms)] = uint32(totalPostings)
	index.PosOffsets[len(terms)] = uint32(totalPositions)
	index.TitlePostingOffsets[len(terms)] = uint32(totalTitlePostings)
	return
}

func (sb *StreamBuilder) fillCSRTables(index *searchpkg.SearchIndex, terms []string, masterInverted map[string]*tempPostings, masterTitleInverted map[string][]uint32) {
	currPosting := 0
	currPos := 0
	currTitlePosting := 0
	for _, term := range terms {
		tp := masterInverted[term]
		lastID := uint32(0)
		for i, docID := range tp.docIDs {
			postingIdx := currPosting + i
			index.DocIDs[postingIdx] = docID - lastID
			lastID = docID
			index.DocPosOffsets[postingIdx] = uint32(currPos)
			posList := tp.positions[i]
			copy(index.Positions[currPos:], posList)
			currPos += len(posList)
		}
		currPosting += len(tp.docIDs)

		if tIDs, ok := masterTitleInverted[term]; ok {
			lastTID := uint32(0)
			for i, tid := range tIDs {
				index.TitleDocIDs[currTitlePosting+i] = tid - lastTID
				lastTID = tid
			}
			currTitlePosting += len(tIDs)
		}
	}
	index.DocPosOffsets[currPosting] = uint32(currPos)
}

// sortPostings sorts docIDs and their parallel positions by docID
func sortPostings(tp *tempPostings) {
	if len(tp.docIDs) <= 1 {
		return
	}
	// Use a small helper struct for sorting
	type item struct {
		id  uint32
		pos []uint32
	}
	items := make([]item, len(tp.docIDs))
	for i := range tp.docIDs {
		items[i] = item{tp.docIDs[i], tp.positions[i]}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].id < items[j].id
	})
	for i := range items {
		tp.docIDs[i] = items[i].id
		tp.positions[i] = items[i].pos
	}
}
