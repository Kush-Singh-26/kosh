package generators

import (
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/tinylib/msgp/msgp"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func GenerateSearchIndex(sink utils.ArtifactSink, outputDir string, indexedPosts []models.IndexedPost) (string, error) {
	totalDocs := len(indexedPosts)
	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8 // Cap concurrency to avoid too many small maps
	}

	// Heuristic: Sum of unique words in each post, adjusted by an overlap factor.
	totalUniqueWordsEst := 0
	for _, ip := range indexedPosts {
		totalUniqueWordsEst += len(ip.PositionalIndex)
	}

	// Factor of 0.5 accounts for common words appearing in multiple posts.
	globalCap := int(float64(totalUniqueWordsEst) * 0.5)
	if globalCap < 500 {
		globalCap = 500
	}

	index := models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         make([]models.PostRecord, totalDocs),
		Inverted:      make(map[string]map[string][]int, globalCap),
		DocLens:       make(map[string]int64, totalDocs),
		StemMap:       make(map[string][]string, globalCap/2),
		TotalDocs:     int64(totalDocs),
		Offsets:       make(map[string]map[string][]int, globalCap),
	}

	// Pre-compute document ID strings to avoid repeated strconv.Itoa in workers
	idStrings := make([]string, totalDocs)
	for i := 0; i < totalDocs; i++ {
		idStrings[i] = strconv.Itoa(i)
	}

	// 1. Parallel collection of posts, doc lengths, and doc-word positions
	var totalLen int64

	type partialResult struct {
		inverted map[string]map[string][]int
		offsets  map[string]map[string][]int
		docLens  map[string]int64
		stemMap  map[string]map[string]bool
		totalLen int64
	}

	results := make([]partialResult, numWorkers)
	var wg sync.WaitGroup

	chunkSize := (totalDocs + numWorkers - 1) / numWorkers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			start := workerID * chunkSize
			end := start + chunkSize
			if end > totalDocs {
				end = totalDocs
			}

			chunkUniqueWords := 0
			for j := start; j < end; j++ {
				chunkUniqueWords += len(indexedPosts[j].PositionalIndex)
			}
			workerCap := int(float64(chunkUniqueWords) * 0.7)

			localInverted := make(map[string]map[string][]int, workerCap)
			localOffsets := make(map[string]map[string][]int, workerCap)
			localDocLens := make(map[string]int64, end-start)
			localStemMap := make(map[string]map[string]bool, workerCap/2)
			var localTotalLen int64

			for j := start; j < end; j++ {
				ip := indexedPosts[j]
				idStr := idStrings[j]
				index.Posts[j] = ip.Record
				localDocLens[idStr] = int64(ip.DocLen)
				localTotalLen += int64(ip.DocLen)

				for word, positions := range ip.PositionalIndex {
					if _, ok := localInverted[word]; !ok {
						localInverted[word] = make(map[string][]int)
					}
					localInverted[word][idStr] = positions
				}

				for word, off := range ip.ByteOffsets {
					if _, ok := localOffsets[word]; !ok {
						localOffsets[word] = make(map[string][]int)
					}
					localOffsets[word][idStr] = off
				}

				for orig, stem := range ip.StemMap {
					if _, ok := localStemMap[stem]; !ok {
						localStemMap[stem] = make(map[string]bool)
					}
					localStemMap[stem][orig] = true
				}
			}
			results[workerID] = partialResult{
				inverted: localInverted,
				offsets:  localOffsets,
				docLens:  localDocLens,
				stemMap:  localStemMap,
				totalLen: localTotalLen,
			}
		}(i)
	}
	wg.Wait()

	// 2. Merge results
	for _, r := range results {
		totalLen += r.totalLen
		for idStr, length := range r.docLens {
			index.DocLens[idStr] = length
		}
		for word, docs := range r.inverted {
			if _, ok := index.Inverted[word]; !ok {
				index.Inverted[word] = docs
			} else {
				for docID, positions := range docs {
					index.Inverted[word][docID] = positions
				}
			}
		}
		for word, docs := range r.offsets {
			if _, ok := index.Offsets[word]; !ok {
				index.Offsets[word] = docs
			} else {
				for docID, off := range docs {
					index.Offsets[word][docID] = off
				}
			}
		}
	}

	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Global stem map merge
	stemMapCap := globalCap / 2
	if stemMapCap < 100 {
		stemMapCap = 100
	}
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

	for stem, originMap := range stemMap {
		origins := make([]string, 0, len(originMap))
		for origin := range originMap {
			origins = append(origins, origin)
		}
		index.StemMap[stem] = origins
	}

	index.NgramIndex = search.BuildNgramIndex(index.Inverted)

	if err := sink.MkdirAll(outputDir); err != nil {
		return "", err
	}

	outputPath := filepath.ToSlash(filepath.Join(outputDir, "search.bin"))
	err := sink.WriteStream(outputPath, func(w io.Writer) error {
		bw := brotli.NewWriterLevel(w, 4)

		mw := msgp.NewWriter(bw)
		if err := index.EncodeMsg(mw); err != nil {
			_ = bw.Close()
			return err
		}

		if err := mw.Flush(); err != nil {
			_ = bw.Close()
			return err
		}

		// Explicitly close to ensure footer is written before underlying stream finishes
		return bw.Close()
	})
	if err != nil {
		return "", err
	}

	return outputPath, nil
}
