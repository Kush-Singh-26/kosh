package index

import (
	"slices"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

type makeItemOptions struct {
	id        uint64
	title     string
	content   string
	terms     []string
	wordFreqs map[string]int
	stemMap   map[string]string
}

func makeItem(opts makeItemOptions) models.IndexedContent {
	posIndex := make(map[string][]uint32)
	byteOffsets := make(map[string][]uint32)
	for i, t := range opts.terms {
		posIndex[t] = []uint32{uint32(i)}
		byteOffsets[t] = []uint32{uint32(i * 4), 4}
	}
	if len(opts.wordFreqs) == 0 {
		opts.wordFreqs = make(map[string]int)
		for _, t := range opts.terms {
			opts.wordFreqs[t]++
		}
	}
	if opts.stemMap == nil {
		opts.stemMap = make(map[string]string)
	}
	return models.IndexedContent{
		Record: models.ContentRecord{
			ID:              opts.id,
			Title:           opts.title,
			NormalizedTitle: opts.title,
			Content:         opts.content,
			Taxonomies:      map[string][]string{"tags": {"go"}},
			NormalizedTaxs:  map[string][]string{"tags": {"go"}},
		},
		DocLen:          len(opts.terms),
		WordFreqs:       opts.wordFreqs,
		PositionalIndex: posIndex,
		ByteOffsets:     byteOffsets,
		StemMap:         opts.stemMap,
	}
}

func TestBuildEmpty(t *testing.T) {
	idx := Build(nil)
	if idx == nil {
		t.Fatal("Build(nil) returned nil")
	}
	if idx.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", idx.TotalItems)
	}
	if len(idx.Items) != 0 {
		t.Errorf("Items len = %d, want 0", len(idx.Items))
	}
	if len(idx.Inverted) != 0 {
		t.Errorf("Inverted len = %d, want 0", len(idx.Inverted))
	}
	if idx.SchemaVersion != models.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", idx.SchemaVersion, models.CurrentSchemaVersion)
	}
}

func TestBuildEmptySlice(t *testing.T) {
	idx := Build([]models.IndexedContent{})
	if idx == nil {
		t.Fatal("Build([]) returned nil")
	}
	if idx.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", idx.TotalItems)
	}
}

func TestBuildSingleItem(t *testing.T) {
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Go Tutorial",
			content: "Learn Go programming",
			terms:   []string{"go", "tutorial", "learn"},
		}),
	}
	idx := Build(items)

	if idx.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", idx.TotalItems)
	}
	if _, ok := idx.Items["1"]; !ok {
		t.Error("Item ID '1' not found in Items map")
	}
	if len(idx.Inverted) == 0 {
		t.Error("Inverted index is empty")
	}
	if _, ok := idx.Inverted["go"]; !ok {
		t.Error("Term 'go' not found in inverted index")
	}
	if _, ok := idx.Inverted["go"]["1"]; !ok {
		t.Error("Item '1' not found under term 'go' in inverted index")
	}
	if idx.AvgDocLen != 3 {
		t.Errorf("AvgDocLen = %f, want 3", idx.AvgDocLen)
	}
	if len(idx.ItemLens) != 1 {
		t.Errorf("ItemLens len = %d, want 1", len(idx.ItemLens))
	}
}

func TestBuildMultipleItems(t *testing.T) {
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Go Concurrency",
			content: "goroutines and channels",
			terms:   []string{"go", "concurrency", "goroutines"},
		}),
		makeItem(makeItemOptions{
			id:      2,
			title:   "Go Testing",
			content: "testing in Go",
			terms:   []string{"go", "testing"},
		}),
		makeItem(makeItemOptions{
			id:      3,
			title:   "Rust Basics",
			content: "learn Rust",
			terms:   []string{"rust", "basics", "learn"},
		}),
	}
	idx := Build(items)

	if idx.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", idx.TotalItems)
	}

	// "go" should appear in items 1 and 2
	goItems, ok := idx.Inverted["go"]
	if !ok {
		t.Fatal("Term 'go' not in inverted index")
	}
	if len(goItems) != 2 {
		t.Errorf("Term 'go' found in %d items, want 2", len(goItems))
	}

	// "rust" should appear only in item 3
	rustItems, ok := idx.Inverted["rust"]
	if !ok {
		t.Fatal("Term 'rust' not in inverted index")
	}
	if len(rustItems) != 1 {
		t.Errorf("Term 'rust' found in %d items, want 1", len(rustItems))
	}

	// Average doc len: (3 + 2 + 3) / 3 = 2.666...
	expectedAvg := float64(3+2+3) / 3.0
	if idx.AvgDocLen != expectedAvg {
		t.Errorf("AvgDocLen = %f, want %f", idx.AvgDocLen, expectedAvg)
	}
}

func TestBuildConsistency(t *testing.T) {
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Item One",
			content: "alpha beta gamma",
			terms:   []string{"alpha", "beta", "gamma"},
		}),
		makeItem(makeItemOptions{
			id:      2,
			title:   "Item Two",
			content: "beta gamma delta",
			terms:   []string{"beta", "gamma", "delta"},
		}),
		makeItem(makeItemOptions{
			id:      3,
			title:   "Item Three",
			content: "gamma delta epsilon",
			terms:   []string{"gamma", "delta", "epsilon"},
		}),
	}

	idx1 := Build(items)
	idx2 := Build(items)

	if idx1.TotalItems != idx2.TotalItems {
		t.Error("TotalItems differ between runs")
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
	// Two items with overlapping terms — inverted index should merge, not overwrite
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Alpha",
			content: "hello world",
			terms:   []string{"hello", "world"},
		}),
		makeItem(makeItemOptions{
			id:      2,
			title:   "Beta",
			content: "hello again",
			terms:   []string{"hello", "again"},
		}),
	}
	idx := Build(items)

	helloItems := idx.Inverted["hello"]
	if len(helloItems) != 2 {
		t.Errorf("Term 'hello' found in %d items, want 2 (merge)", len(helloItems))
	}
	if _, ok := helloItems["1"]; !ok {
		t.Error("Item 1 not found under 'hello'")
	}
	if _, ok := helloItems["2"]; !ok {
		t.Error("Item 2 not found under 'hello'")
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
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Running",
			content: "running runs",
			terms:   []string{"running", "runs"},
			stemMap: map[string]string{
				"running": "run",
				"runs":    "run",
			},
		}),
		makeItem(makeItemOptions{
			id:      2,
			title:   "Runner",
			content: "runner ran",
			terms:   []string{"runner", "ran"},
			stemMap: map[string]string{
				"runner": "run",
				"ran":    "run",
			},
		}),
	}
	idx := Build(items)

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
	items := []models.IndexedContent{
		makeItem(makeItemOptions{
			id:      1,
			title:   "Ngrams",
			content: "ngram testing",
			terms:   []string{"ngram", "testing"},
		}),
	}
	idx := Build(items)

	if idx.NgramIndex == nil {
		t.Fatal("NgramIndex is nil")
	}
	if len(idx.NgramIndex) == 0 {
		t.Error("NgramIndex is empty for non-empty input")
	}
}
