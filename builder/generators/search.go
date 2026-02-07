package generators

import (
	"compress/gzip"
	"encoding/gob"
	"path/filepath"

	"github.com/spf13/afero"

	"my-ssg/builder/models"
)

func GenerateSearchIndex(destFs afero.Fs, outputDir string, indexedPosts []models.IndexedPost) error {
	totalDocs := len(indexedPosts)
	// Heuristic: estimate 100 unique words per post
	estimatedUniqueWords := totalDocs * 100

	index := models.SearchIndex{
		Posts:    make([]models.PostRecord, totalDocs),
		Inverted: make(map[string]map[int]int, estimatedUniqueWords),
		DocLens:  make(map[int]int, totalDocs),
	}

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
	}

	index.TotalDocs = len(indexedPosts)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Save to compressed binary file
	if err := destFs.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Use .bin.gz extension or just .bin?
	// The plan said .bin.gz and client inflates it.
	// But `search.bin` is what the client expects currently.
	// I'll change it to `search.bin` but compressed content, or `search.bin.gz`.
	// If I change filename, I must update WASM.
	// I'll use `search.bin` for now but gzip it. The client will need to know.
	// Actually, browsers can handle gzip via Content-Encoding if served correctly,
	// but for WASM manual fetch, explicit decompression is better.
	// I'll name it `search.bin` to minimize `run.go` changes for now,
	// BUT `run.go` calls this function. I can pass `public/search.bin`.
	// Let's use `search.bin` but containing gzipped data.

	file, err := destFs.Create(filepath.Join(outputDir, "search.bin"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	enc := gob.NewEncoder(gw)
	return enc.Encode(index)
}
