package index

import (
	"slices"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func makePost(id uint64, title, content string, terms []string, wordFreqs map[string]int, stemMap map[string]string) models.IndexedPost {
	posIndex := make(map[string][]uint32)
	byteOffsets := make(map[string][]uint32)
	for i, t := range terms {
		posIndex[t] = []uint32{uint32(i)}
		byteOffsets[t] = []uint32{uint32(i * 4), 4}
	}
	if len(wordFreqs) == 0 {
		wordFreqs = make(map[string]int)
		for _, t := range terms {
			wordFreqs[t]++
		}
	}
	if stemMap == nil {
		stemMap = make(map[string]string)
	}
	return models.IndexedPost{
		Record: models.PostRecord{
			ID:              id,
			Title:           title,
			NormalizedTitle: title,
			Content:         content,
			Tags:            []string{"go"},
			NormalizedTags:  []string{"go"},
		},
		DocLen:          len(terms),
		WordFreqs:       wordFreqs,
		PositionalIndex: posIndex,
		ByteOffsets:     byteOffsets,
		StemMap:         stemMap,
	}
}

func TestBuildEmpty(t *testing.T) {
	idx := Build(nil)
	if idx == nil {
		t.Fatal("Build(nil) returned nil")
	}
	if idx.TotalDocs != 0 {
		t.Errorf("TotalDocs = %d, want 0", idx.TotalDocs)
	}
	if len(idx.Posts) != 0 {
		t.Errorf("Posts len = %d, want 0", len(idx.Posts))
	}
	if len(idx.Inverted) != 0 {
		t.Errorf("Inverted len = %d, want 0", len(idx.Inverted))
	}
	if idx.SchemaVersion != models.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", idx.SchemaVersion, models.CurrentSchemaVersion)
	}
}

func TestBuildEmptySlice(t *testing.T) {
	idx := Build([]models.IndexedPost{})
	if idx == nil {
		t.Fatal("Build([]) returned nil")
	}
	if idx.TotalDocs != 0 {
		t.Errorf("TotalDocs = %d, want 0", idx.TotalDocs)
	}
}

func TestBuildSinglePost(t *testing.T) {
	posts := []models.IndexedPost{
		makePost(1, "Go Tutorial", "Learn Go programming", []string{"go", "tutorial", "learn"}, nil, nil),
	}
	idx := Build(posts)

	if idx.TotalDocs != 1 {
		t.Errorf("TotalDocs = %d, want 1", idx.TotalDocs)
	}
	if _, ok := idx.Posts["1"]; !ok {
		t.Error("Post ID '1' not found in Posts map")
	}
	if len(idx.Inverted) == 0 {
		t.Error("Inverted index is empty")
	}
	if _, ok := idx.Inverted["go"]; !ok {
		t.Error("Term 'go' not found in inverted index")
	}
	if _, ok := idx.Inverted["go"]["1"]; !ok {
		t.Error("Post '1' not found under term 'go' in inverted index")
	}
	if idx.AvgDocLen != 3 {
		t.Errorf("AvgDocLen = %f, want 3", idx.AvgDocLen)
	}
	if len(idx.DocLens) != 1 {
		t.Errorf("DocLens len = %d, want 1", len(idx.DocLens))
	}
}

func TestBuildMultiplePosts(t *testing.T) {
	posts := []models.IndexedPost{
		makePost(1, "Go Concurrency", "goroutines and channels", []string{"go", "concurrency", "goroutines"}, nil, nil),
		makePost(2, "Go Testing", "testing in Go", []string{"go", "testing"}, nil, nil),
		makePost(3, "Rust Basics", "learn Rust", []string{"rust", "basics", "learn"}, nil, nil),
	}
	idx := Build(posts)

	if idx.TotalDocs != 3 {
		t.Errorf("TotalDocs = %d, want 3", idx.TotalDocs)
	}

	// "go" should appear in posts 1 and 2
	goDocs, ok := idx.Inverted["go"]
	if !ok {
		t.Fatal("Term 'go' not in inverted index")
	}
	if len(goDocs) != 2 {
		t.Errorf("Term 'go' found in %d docs, want 2", len(goDocs))
	}

	// "rust" should appear only in post 3
	rustDocs, ok := idx.Inverted["rust"]
	if !ok {
		t.Fatal("Term 'rust' not in inverted index")
	}
	if len(rustDocs) != 1 {
		t.Errorf("Term 'rust' found in %d docs, want 1", len(rustDocs))
	}

	// Average doc len: (3 + 2 + 3) / 3 = 2.666...
	expectedAvg := float64(3+2+3) / 3.0
	if idx.AvgDocLen != expectedAvg {
		t.Errorf("AvgDocLen = %f, want %f", idx.AvgDocLen, expectedAvg)
	}
}

func TestBuildConsistency(t *testing.T) {
	posts := []models.IndexedPost{
		makePost(1, "Post One", "alpha beta gamma", []string{"alpha", "beta", "gamma"}, nil, nil),
		makePost(2, "Post Two", "beta gamma delta", []string{"beta", "gamma", "delta"}, nil, nil),
		makePost(3, "Post Three", "gamma delta epsilon", []string{"gamma", "delta", "epsilon"}, nil, nil),
	}

	idx1 := Build(posts)
	idx2 := Build(posts)

	if idx1.TotalDocs != idx2.TotalDocs {
		t.Error("TotalDocs differ between runs")
	}
	if idx1.AvgDocLen != idx2.AvgDocLen {
		t.Error("AvgDocLen differ between runs")
	}
	if len(idx1.Inverted) != len(idx2.Inverted) {
		t.Error("Inverted index sizes differ between runs")
	}
	for term, docs1 := range idx1.Inverted {
		docs2, ok := idx2.Inverted[term]
		if !ok {
			t.Errorf("Term %q missing in second run", term)
			continue
		}
		if len(docs1) != len(docs2) {
			t.Errorf("Term %q: doc count %d vs %d", term, len(docs1), len(docs2))
		}
		for docID := range docs1 {
			if _, ok := docs2[docID]; !ok {
				t.Errorf("Term %q: doc %q missing in second run", term, docID)
			}
		}
	}
}

func TestBuildTermIndexMerging(t *testing.T) {
	// Two posts with overlapping terms — inverted index should merge, not overwrite
	posts := []models.IndexedPost{
		makePost(1, "Alpha", "hello world", []string{"hello", "world"}, nil, nil),
		makePost(2, "Beta", "hello again", []string{"hello", "again"}, nil, nil),
	}
	idx := Build(posts)

	helloDocs := idx.Inverted["hello"]
	if len(helloDocs) != 2 {
		t.Errorf("Term 'hello' found in %d docs, want 2 (merge)", len(helloDocs))
	}
	if _, ok := helloDocs["1"]; !ok {
		t.Error("Post 1 not found under 'hello'")
	}
	if _, ok := helloDocs["2"]; !ok {
		t.Error("Post 2 not found under 'hello'")
	}

	// "world" should only be in post 1
	worldDocs := idx.Inverted["world"]
	if len(worldDocs) != 1 {
		t.Errorf("Term 'world' found in %d docs, want 1", len(worldDocs))
	}

	// "again" should only be in post 2
	againDocs := idx.Inverted["again"]
	if len(againDocs) != 1 {
		t.Errorf("Term 'again' found in %d docs, want 1", len(againDocs))
	}
}

func TestBuildStemMapMerging(t *testing.T) {
	posts := []models.IndexedPost{
		makePost(1, "Running", "running runs", []string{"running", "runs"}, nil, map[string]string{
			"running": "run",
			"runs":    "run",
		}),
		makePost(2, "Runner", "runner ran", []string{"runner", "ran"}, nil, map[string]string{
			"runner": "run",
			"ran":    "run",
		}),
	}
	idx := Build(posts)

	stemmed, ok := idx.StemMap["run"]
	if !ok {
		t.Fatal("Stem 'run' not found in StemMap")
	}
	expected := []string{"running", "runs", "runner", "ran"}
	slices.Sort(stemmed)
	slices.Sort(expected)
	if len(stemmed) != len(expected) {
		t.Errorf("StemMap[run] has %d entries, want %d", len(stemmed), len(expected))
	}
	for i, v := range expected {
		if i >= len(stemmed) || stemmed[i] != v {
			t.Errorf("StemMap[run][%d] = %q, want %q", i, stemmed[i], v)
		}
	}
}

func TestBuildNgramIndex(t *testing.T) {
	posts := []models.IndexedPost{
		makePost(1, "Ngrams", "ngram testing", []string{"ngram", "testing"}, nil, nil),
	}
	idx := Build(posts)

	if idx.NgramIndex == nil {
		t.Fatal("NgramIndex is nil")
	}
	if len(idx.NgramIndex) == 0 {
		t.Error("NgramIndex is empty for non-empty input")
	}
}
