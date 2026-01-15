package generators

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"

	"my-ssg/builder/models"
	"my-ssg/builder/search"
)

func GenerateSearchIndex(outputDir string, posts []models.PostRecord) error {
	index := models.SearchIndex{
		Posts:    posts,
		Inverted: make(map[string]map[int]int),
		DocLens:  make(map[int]int),
	}

	totalLen := 0
	for i, post := range posts {
		// Combine Title, Description, Tags, and Content for indexing
		fullText := strings.ToLower(post.Title + " " + post.Description + " " + strings.Join(post.Tags, " ") + " " + post.Content)
		words := search.Tokenize(fullText)

		docLen := len(words)
		index.DocLens[i] = docLen
		totalLen += docLen

		for _, word := range words {
			if len(word) < 2 {
				continue
			}
			if index.Inverted[word] == nil {
				index.Inverted[word] = make(map[int]int)
			}
			index.Inverted[word][i]++
		}
	}

	index.TotalDocs = len(posts)
	if index.TotalDocs > 0 {
		index.AvgDocLen = float64(totalLen) / float64(index.TotalDocs)
	}

	// Save to binary file
	os.MkdirAll(outputDir, 0755)
	file, err := os.Create(filepath.Join(outputDir, "search.bin"))
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)
	return enc.Encode(index)
}
