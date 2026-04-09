package index

import (
	"context"
	"log/slog"
	"maps"
	"runtime"
	"strconv"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

const (
	maxIndexWorkers = 8
	globalCapScale  = 0.5
	minGlobalCap    = 500
	workerCapScale  = 0.7
	minWorkerCap    = 64
	perWordMapCap   = 4
	minStemMapCap   = 100
	decimalBase     = 10
)

type partialResult struct {
	posts    map[string]models.PostRecord
	inverted map[string]map[string][]uint32
	offsets  map[string]map[string][]uint32
	docLens  map[string]int64
	stemMap  map[string]map[string]bool
	totalLen int64
}

func emptySearchIndex() *models.SearchIndex {
	return &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         make(map[string]models.PostRecord),
		Inverted:      make(map[string]map[string][]uint32),
		DocLens:       make(map[string]int64),
		StemMap:       make(map[string][]string),
		TotalDocs:     0,
		Offsets:       make(map[string]map[string][]uint32),
		AvgDocLen:     0,
	}
}

func computeGlobalCap(indexedPosts []models.IndexedPost) int {
	totalUniqueWordsEst := 0
	for _, ip := range indexedPosts {
		totalUniqueWordsEst += len(ip.PositionalIndex)
	}
	return max(int(float64(totalUniqueWordsEst)*globalCapScale), minGlobalCap)
}

func buildPartialResults(indexedPosts []models.IndexedPost, numWorkers int) []partialResult {
	totalDocs := len(indexedPosts)
	results := make([]partialResult, numWorkers)
	var wg sync.WaitGroup
	chunkSize := (totalDocs + numWorkers - 1) / numWorkers

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := i
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    slog.Default(),
			Operation: "search index build",
			Fn: func() error {
				start := workerID * chunkSize
				end := min(start+chunkSize, totalDocs)

				chunkUniqueWords := 0
				for j := start; j < end; j++ {
					chunkUniqueWords += len(indexedPosts[j].PositionalIndex)
				}
				workerCap := max(int(float64(chunkUniqueWords)*workerCapScale), minWorkerCap)

				localPosts := make(map[string]models.PostRecord, end-start)
				localInverted := make(map[string]map[string][]uint32, workerCap)
				localOffsets := make(map[string]map[string][]uint32, workerCap)
				localDocLens := make(map[string]int64, end-start)
				localStemMap := make(map[string]map[string]bool, workerCap/2)
				var localTotalLen int64

				for j := start; j < end; j++ {
					ip := indexedPosts[j]
					idStr := strconv.FormatUint(ip.Record.ID, decimalBase)
					localPosts[idStr] = ip.Record
					localDocLens[idStr] = int64(ip.DocLen)
					localTotalLen += int64(ip.DocLen)

					for word, positions := range ip.PositionalIndex {
						if _, ok := localInverted[word]; !ok {
							localInverted[word] = make(map[string][]uint32, perWordMapCap)
						}
						localInverted[word][idStr] = positions
					}

					for word, off := range ip.ByteOffsets {
						if _, ok := localOffsets[word]; !ok {
							localOffsets[word] = make(map[string][]uint32, perWordMapCap)
						}
						localOffsets[word][idStr] = off
					}

					for orig, stem := range ip.StemMap {
						if _, ok := localStemMap[stem]; !ok {
							localStemMap[stem] = make(map[string]bool, perWordMapCap)
						}
						localStemMap[stem][orig] = true
					}
				}
				results[workerID] = partialResult{
					posts:    localPosts,
					inverted: localInverted,
					offsets:  localOffsets,
					docLens:  localDocLens,
					stemMap:  localStemMap,
					totalLen: localTotalLen,
				}
				return nil
			},
			Cleanup: wg.Done,
		})
	}
	wg.Wait()
	return results
}

func mergePartialResults(index *models.SearchIndex, results []partialResult) int64 {
	var totalLen int64
	for _, r := range results {
		totalLen += r.totalLen
		maps.Copy(index.Posts, r.posts)
		maps.Copy(index.DocLens, r.docLens)
		for word, docs := range r.inverted {
			if _, ok := index.Inverted[word]; !ok {
				index.Inverted[word] = docs
			} else {
				maps.Copy(index.Inverted[word], docs)
			}
		}
		for word, docs := range r.offsets {
			if _, ok := index.Offsets[word]; !ok {
				index.Offsets[word] = docs
			} else {
				maps.Copy(index.Offsets[word], docs)
			}
		}
	}
	return totalLen
}

func buildStemOrigins(results []partialResult, globalCap int) map[string][]string {
	stemMapCap := max(globalCap/2, minStemMapCap)
	stemMap := make(map[string]map[string]bool, stemMapCap)
	for _, r := range results {
		for stem, origins := range r.stemMap {
			if _, ok := stemMap[stem]; !ok {
				stemMap[stem] = origins
			} else {
				for orig := range origins {
					stemMap[stem][orig] = true
				}
			}
		}
	}

	flattened := make(map[string][]string, len(stemMap))
	for stem, originMap := range stemMap {
		origins := make([]string, 0, len(originMap))
		for origin := range originMap {
			origins = append(origins, origin)
		}
		flattened[stem] = origins
	}
	return flattened
}

// Build constructs an in-memory search index from a list of indexed posts.
func Build(indexedPosts []models.IndexedPost) *models.SearchIndex {
	totalDocs := len(indexedPosts)

	if totalDocs == 0 {
		return emptySearchIndex()
	}

	numWorkers := min(runtime.NumCPU(), maxIndexWorkers)

	globalCap := computeGlobalCap(indexedPosts)

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         make(map[string]models.PostRecord, totalDocs),
		Inverted:      make(map[string]map[string][]uint32, globalCap),
		DocLens:       make(map[string]int64, totalDocs),
		StemMap:       make(map[string][]string, globalCap/2),
		TotalDocs:     int64(totalDocs),
		Offsets:       make(map[string]map[string][]uint32, globalCap),
	}

	results := buildPartialResults(indexedPosts, numWorkers)
	totalLen := mergePartialResults(index, results)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	index.StemMap = buildStemOrigins(results, globalCap)

	index.NgramIndex = core.BuildNgramIndex(index.Inverted)
	return index
}
