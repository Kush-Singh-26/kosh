package generators

import (
	"io"
	"maps"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/tinylib/msgp/msgp"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

func GenerateSearchIndex(sink fspkg.ArtifactSink, indexedPosts []models.IndexedPost) (string, error) {
	totalDocs := len(indexedPosts)
	outputDir := sink.GetOutputDir()

	// Handle empty input - write empty index
	if indexedPosts == nil || totalDocs == 0 {
		outputPath := "search.bin"

		// Create empty search index with valid schema
		emptyIndex := models.SearchIndex{
			SchemaVersion: models.CurrentSchemaVersion,
			Posts:         make(map[string]models.PostRecord),
			Inverted:      make(map[string]map[string][]uint32),
			DocLens:       make(map[string]int64),
			StemMap:       make(map[string][]string),
			TotalDocs:     0,
			Offsets:       make(map[string]map[string][]uint32),
			AvgDocLen:     0,
		}

		err := sink.WriteStream(outputPath, func(w io.Writer) error {
			bw := brotli.NewWriterLevel(w, 4)
			mw := msgp.NewWriter(bw)
			if err := emptyIndex.EncodeMsg(mw); err != nil {
				_ = bw.Close()
				return err
			}
			if err := mw.Flush(); err != nil {
				_ = bw.Close()
				return err
			}
			return bw.Close()
		})
		if err != nil {
			return "", err
		}
		return filepath.Join(outputDir, outputPath), nil
	}

	numWorkers := min(runtime.NumCPU(),
		// Cap concurrency to avoid too many small maps
		8)

	// Heuristic: Sum of unique words in each post, adjusted by an overlap factor.
	totalUniqueWordsEst := 0
	for _, ip := range indexedPosts {
		totalUniqueWordsEst += len(ip.PositionalIndex)
	}

	// Factor of 0.5 accounts for common words appearing in multiple posts.
	globalCap := max(int(float64(totalUniqueWordsEst)*0.5), 500)

	index := models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         make(map[string]models.PostRecord, totalDocs),
		Inverted:      make(map[string]map[string][]uint32, globalCap),
		DocLens:       make(map[string]int64, totalDocs),
		StemMap:       make(map[string][]string, globalCap/2),
		TotalDocs:     int64(totalDocs),
		Offsets:       make(map[string]map[string][]uint32, globalCap),
	}

	// 1. Parallel collection of posts, doc lengths, and doc-word positions
	var totalLen int64

	type partialResult struct {
		posts    map[string]models.PostRecord
		inverted map[string]map[string][]uint32
		offsets  map[string]map[string][]uint32
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
			end := min(start+chunkSize, totalDocs)

			chunkUniqueWords := 0
			for j := start; j < end; j++ {
				chunkUniqueWords += len(indexedPosts[j].PositionalIndex)
			}
			workerCap := max(int(float64(chunkUniqueWords)*0.7), 64)

			localPosts := make(map[string]models.PostRecord, end-start)
			localInverted := make(map[string]map[string][]uint32, workerCap)
			localOffsets := make(map[string]map[string][]uint32, workerCap)
			localDocLens := make(map[string]int64, end-start)
			localStemMap := make(map[string]map[string]bool, workerCap/2)
			var localTotalLen int64

			for j := start; j < end; j++ {
				ip := indexedPosts[j]
				id := ip.Record.ID
				idStr := strconv.FormatUint(id, 10)
				localPosts[idStr] = ip.Record
				localDocLens[idStr] = int64(ip.DocLen)
				localTotalLen += int64(ip.DocLen)

				for word, positions := range ip.PositionalIndex {
					if _, ok := localInverted[word]; !ok {
						localInverted[word] = make(map[string][]uint32, 4)
					}
					localInverted[word][idStr] = models.EncodePositions(positions)
				}

				for word, off := range ip.ByteOffsets {
					if _, ok := localOffsets[word]; !ok {
						localOffsets[word] = make(map[string][]uint32, 4)
					}
					localOffsets[word][idStr] = models.EncodeOffsets(off)
				}

				for orig, stem := range ip.StemMap {
					if _, ok := localStemMap[stem]; !ok {
						localStemMap[stem] = make(map[string]bool, 4)
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
		}(i)
	}
	wg.Wait()

	// 2. Merge results
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

	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Global stem map merge
	stemMapCap := max(globalCap/2, 100)
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

	index.NgramIndex = core.BuildNgramIndex(index.Inverted)

	outputPath := "search.bin"
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

	return filepath.Join(outputDir, outputPath), nil
}
