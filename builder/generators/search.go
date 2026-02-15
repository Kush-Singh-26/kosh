package generators

import (
	"compress/gzip"
	"path/filepath"

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
				postMap = make(map[int]int)
				index.Inverted[word] = postMap
			}
			postMap[i] = freq
		}

		// Build stem map for fuzzy matching
		stemmed, originals := analyzer.AnalyzeWithOriginals(ip.Record.Content)
		for j, stem := range stemmed {
			if j < len(originals) {
				orig := originals[j]
				if stem != orig {
					existing := index.StemMap[stem]
					found := false
					for _, e := range existing {
						if e == orig {
							found = true
							break
						}
					}
					if !found {
						index.StemMap[stem] = append(index.StemMap[stem], orig)
					}
				}
			}
		}
	}

	index.TotalDocs = len(indexedPosts)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

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
