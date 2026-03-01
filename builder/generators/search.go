package generators

import (
	"compress/gzip"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/spf13/afero"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

func GenerateSearchIndex(destFs afero.Fs, outputDir string, indexedPosts []models.IndexedPost) error {
	totalDocs := len(indexedPosts)
	estimatedUniqueWords := totalDocs * 100

	index := models.SearchIndex{
		Posts:    make([]models.PostRecord, totalDocs),
		Inverted: make(map[string]map[int]int, estimatedUniqueWords),
		DocLens:  make(map[int]int, totalDocs),
		StemMap:  make(map[string][]string),
	}

	analyzer := search.NewAnalyzer(true, true)

	totalLen := 0
	for i, ip := range indexedPosts {
		index.Posts[i] = ip.Record
		index.DocLens[i] = ip.DocLen
		totalLen += ip.DocLen

		for word, freq := range ip.WordFreqs {
			postMap, ok := index.Inverted[word]
			if !ok {
				postMap = make(map[int]int, 4)
				index.Inverted[word] = postMap
			}
			postMap[i] = freq
		}
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 4 {
		numWorkers = 4
	}
	if totalDocs < 10 {
		numWorkers = 1
	}

	type stemResult struct {
		stem   string
		origin string
	}

	stemChan := make(chan stemResult, totalDocs*10)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := workerID; i < totalDocs; i += numWorkers {
				ip := indexedPosts[i]
				stemmed, originals := analyzer.AnalyzeWithOriginals(ip.Record.Content)
				for j, stem := range stemmed {
					if j < len(originals) {
						orig := originals[j]
						if stem != orig {
							stemChan <- stemResult{stem: stem, origin: orig}
						}
					}
				}
			}
		}(w)
	}

	go func() {
		wg.Wait()
		close(stemChan)
	}()

	stemMap := make(map[string]map[string]bool)
	for sr := range stemChan {
		if _, ok := stemMap[sr.stem]; !ok {
			stemMap[sr.stem] = make(map[string]bool)
		}
		stemMap[sr.stem][sr.origin] = true
	}

	for stem, originMap := range stemMap {
		for origin := range originMap {
			index.StemMap[stem] = append(index.StemMap[stem], origin)
		}
	}

	index.TotalDocs = len(indexedPosts)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	index.NgramIndex = search.BuildNgramIndex(index.Inverted)

	if err := destFs.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	file, err := destFs.Create(filepath.Join(outputDir, "search.bin"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer func() { _ = gw.Close() }()

	enc := msgpack.NewEncoder(gw)
	return enc.Encode(&index)
}
