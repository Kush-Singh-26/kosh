package generators

import (
	"io"
	"path/filepath"

	"github.com/tinylib/msgp/msgp"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)

// GenerateSearchIndex builds and writes the search index to the output directory.
func GenerateSearchIndex(sink fspkg.ArtifactSink, indexedPosts []searchpkg.IndexedContent, ranking searchpkg.SearchRankingConfig) (string, int64, error) {
	idx := index.Build(indexedPosts)
	idx.Ranking = ranking
	return GenerateSearchIndexFromObject(sink, idx)
}

// GenerateSearchIndexFromObject writes a pre-built search index to the sink.
func GenerateSearchIndexFromObject(sink fspkg.ArtifactSink, idx *searchpkg.SearchIndex) (string, int64, error) {
	outputDir := sink.GetOutputDir()

	outputPath := "search.bin"
	var size int64
	err := sink.WriteStream(outputPath, func(w io.Writer) error {
		mw := msgp.NewWriter(w)
		if err := idx.EncodeMsg(mw); err != nil {
			return err
		}

		return mw.Flush()
	})
	if err != nil {
		return "", 0, err
	}

	if info, err := sink.Stat(outputPath); err == nil {
		size = info.Size()
	}

	return filepath.Join(outputDir, outputPath), size, nil
}
