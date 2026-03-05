package generators

import (
	"compress/gzip"
	"log/slog"
	"path/filepath"
	"strconv"

	"github.com/spf13/afero"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

func GenerateSearchIndex(destFs afero.Fs, outputDir string, indexedPosts []models.IndexedPost) (string, error) {
	totalDocs := len(indexedPosts)
	estimatedUniqueWords := totalDocs * 100

	index := models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         make([]models.PostRecord, totalDocs),
		Inverted:      make(map[string]map[string][]int, estimatedUniqueWords),
		DocLens:       make(map[string]int64, totalDocs),
		StemMap:       make(map[string][]string),
	}

	totalLen := 0
	for i, ip := range indexedPosts {
		idStr := strconv.Itoa(i)
		index.Posts[i] = ip.Record
		index.Posts[i].Content = ip.Record.Content // Explicitly ensure Content is set
		index.DocLens[idStr] = int64(ip.DocLen)
		totalLen += ip.DocLen

		// Populate unified inverted index with positions
		for word, positions := range ip.PositionalIndex {
			postMap, ok := index.Inverted[word]
			if !ok {
				postMap = make(map[string][]int, 4)
				index.Inverted[word] = postMap
			}
			postMap[idStr] = positions
		}
	}

	index.TotalDocs = int64(len(indexedPosts))
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Populate global stem map from pre-computed per-post mappings
	stemMap := make(map[string]map[string]bool)
	for _, ip := range indexedPosts {
		for orig, stem := range ip.StemMap {
			if _, ok := stemMap[stem]; !ok {
				stemMap[stem] = make(map[string]bool)
			}
			stemMap[stem][orig] = true
		}
	}

	for stem, originMap := range stemMap {
		for origin := range originMap {
			index.StemMap[stem] = append(index.StemMap[stem], origin)
		}
	}

	index.NgramIndex = search.BuildNgramIndex(index.Inverted)

	if err := destFs.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}

	outputPath := filepath.ToSlash(filepath.Join(outputDir, "search.bin"))
	file, err := destFs.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("Failed to close search index file", "error", err)
		}
	}()

	gw := gzip.NewWriter(file)
	defer func() {
		if err := gw.Close(); err != nil {
			slog.Error("Failed to close gzip writer", "error", err)
		}
	}()

	enc := msgpack.NewEncoder(gw)
	if err := enc.Encode(&index); err != nil {
		return "", err
	}

	return outputPath, nil
}
