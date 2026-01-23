package generators

import (
	"encoding/gob"
	"os"
	"path/filepath"

	"my-ssg/builder/models"
)

func GenerateSearchIndex(outputDir string, indexedPosts []models.IndexedPost) error {
	index := models.SearchIndex{
		Posts:    make([]models.PostRecord, len(indexedPosts)),
		Inverted: make(map[string]map[int]int),
		DocLens:  make(map[int]int),
	}

	totalLen := 0
	for i, ip := range indexedPosts {
		index.Posts[i] = ip.Record
		index.DocLens[i] = ip.DocLen
		totalLen += ip.DocLen

		for word, freq := range ip.WordFreqs {
			if index.Inverted[word] == nil {
				index.Inverted[word] = make(map[int]int)
			}
			index.Inverted[word][i] = freq
		}
	}

	index.TotalDocs = len(indexedPosts)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Save to binary file
	_ = os.MkdirAll(outputDir, 0755)
	file, err := os.Create(filepath.Join(outputDir, "search.bin"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	enc := gob.NewEncoder(file)
	return enc.Encode(index)
}
