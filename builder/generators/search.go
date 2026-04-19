package generators

import (
	"io"
	"path/filepath"

	"github.com/andybalholm/brotli"
	"github.com/tinylib/msgp/msgp"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)

const searchIndexBrotliLevel = 4

// GenerateSearchIndex builds and writes the search index to the output directory.
func GenerateSearchIndex(sink fspkg.ArtifactSink, indexedPosts []models.IndexedContent, ranking models.SearchRankingConfig) (string, int64, error) {
	idx := index.Build(indexedPosts)
	idx.Ranking = ranking
	return GenerateSearchIndexFromObject(sink, idx)
}

// GenerateSearchIndexFromObject writes a pre-built search index to the sink.
func GenerateSearchIndexFromObject(sink fspkg.ArtifactSink, idx *models.SearchIndex) (string, int64, error) {
	outputDir := sink.GetOutputDir()

	outputPath := "search.bin"
	var size int64
	err := sink.WriteStream(outputPath, func(w io.Writer) error {
		bw := brotli.NewWriterLevel(w, searchIndexBrotliLevel)

		mw := msgp.NewWriter(bw)
		if err := idx.EncodeMsg(mw); err != nil {
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
		return "", 0, err
	}

	if info, err := sink.Stat(outputPath); err == nil {
		size = info.Size()
	}

	return filepath.Join(outputDir, outputPath), size, nil
}
