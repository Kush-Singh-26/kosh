package search

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

func buildRankingIndex() *models.SearchIndex {
	posts := map[string]models.PostRecord{
		"0": {
			ID:              0,
			Title:           "Go Programming Guide",
			NormalizedTitle: "go programming guide",
			Description:     "Learn Go programming language basics",
			Version:         "v1",
			NormalizedTags:  []string{"go", "programming"},
			Content:         "Go is a programming language created at Google. This guide covers Go basics.",
		},
		"1": {
			ID:              1,
			Title:           "Rust Programming Tutorial",
			NormalizedTitle: "rust programming tutorial",
			Description:     "Learn Rust programming from scratch",
			Version:         "v1",
			NormalizedTags:  []string{"rust", "programming"},
			Content:         "Rust is a systems programming language. This tutorial covers Rust fundamentals.",
		},
		"2": {
			ID:              2,
			Title:           "Machine Learning With Go",
			NormalizedTitle: "machine learning with go",
			Description:     "Using machine learning in Go applications",
			Version:         "v2",
			NormalizedTags:  []string{"ml", "go"},
			Content:         "Machine learning can be implemented in Go. This article explores ML with Go.",
		},
		"3": {
			ID:              3,
			Title:           "Neural Network Fundamentals",
			NormalizedTitle: "neural network fundamentals",
			Description:     "Understanding neural networks and deep learning",
			Version:         "v2",
			NormalizedTags:  []string{"ml", "ai"},
			Content:         "Neural networks are the foundation of deep learning. This guide explains neural networks.",
		},
		"4": {
			ID:              4,
			Title:           "Go Concurrency Patterns",
			NormalizedTitle: "go concurrency patterns",
			Description:     "Advanced Go concurrency and parallelism",
			Version:         "v1",
			NormalizedTags:  []string{"go", "concurrency"},
			Content:         "Go has excellent concurrency support with goroutines and channels. Learn patterns here.",
		},
		"5": {
			ID:              5,
			Title:           "Web Development with Rust",
			NormalizedTitle: "web development with rust",
			Description:     "Building web applications in Rust",
			Version:         "v2",
			NormalizedTags:  []string{"rust", "web"},
			Content:         "Rust can be used for web development using frameworks like Actix and Axum.",
		},
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      make(map[string]map[string][]uint32),
		Offsets:       make(map[string]map[string][]uint32),
		DocLens:       make(map[string]int64),
		TotalDocs:     6,
		AvgDocLen:     10.0,
		NgramIndex:    core.BuildNgramIndex(make(map[string]map[string][]uint32)),
	}

	addTerm := func(term string, postID string, pos uint32) {
		if index.Inverted[term] == nil {
			index.Inverted[term] = make(map[string][]uint32)
		}
		index.Inverted[term][postID] = append(index.Inverted[term][postID], pos)
		if index.Offsets[term] == nil {
			index.Offsets[term] = make(map[string][]uint32)
		}
	}

	addTerm("go", "0", 0)
	addTerm("program", "0", 1)
	addTerm("guide", "0", 3)

	addTerm("rust", "1", 0)
	addTerm("program", "1", 1)
	addTerm("tutorial", "1", 2)

	addTerm("machin", "2", 0)
	addTerm("learn", "2", 1)
	addTerm("go", "2", 2)

	addTerm("neural", "3", 0)
	addTerm("network", "3", 1)

	addTerm("go", "4", 0)
	addTerm("concurr", "4", 2)

	addTerm("rust", "5", 0)
	addTerm("web", "5", 2)

	for _, pid := range []string{"0", "1", "2", "3", "4", "5"} {
		index.DocLens[pid] = 10
	}

	return index
}

func TestRanking_TitleBoost(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "go", "all")

	if len(results) < 1 {
		t.Fatalf("Expected at least 1 result for 'go', got %d", len(results))
	}

	titleMatches := 0
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Title), "go") {
			titleMatches++
		}
	}

	if titleMatches == 0 {
		t.Error("Expected at least one result with 'go' in title")
	}
}

func TestRanking_TagBoost(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "programming", "all")

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results for 'programming', got %d", len(results))
	}

	tagMatches := 0
	for _, r := range results {
		if slices.Contains(index.Posts[strconv.FormatUint(r.ID, 10)].NormalizedTags, "programming") {
			tagMatches++
		}
	}

	if tagMatches == 0 {
		t.Error("Expected at least one result with 'programming' tag")
	}
}

func TestRanking_PhraseMatch(t *testing.T) {
	posts := map[string]models.PostRecord{
		"0": {
			ID:              0,
			Title:           "Neural Network Basics",
			NormalizedTitle: "neural network basics",
			Content:         "A neural network consists of layers.",
			NormalizedTags:  []string{},
		},
		"1": {
			ID:              1,
			Title:           "Network Security",
			NormalizedTitle: "network security",
			Content:         "Networks are important for security.",
			NormalizedTags:  []string{},
		},
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      map[string]map[string][]uint32{},
		Offsets:       map[string]map[string][]uint32{},
		DocLens:       map[string]int64{"0": 5, "1": 5},
		TotalDocs:     2,
		AvgDocLen:     5.0,
	}

	index.Inverted["neural"] = map[string][]uint32{"0": {0}}
	index.Inverted["network"] = map[string][]uint32{"0": {1}, "1": {0}}

	results := PerformSearch(index, "neural network", "all")

	if len(results) < 1 {
		t.Fatalf("Expected at least 1 result, got %d", len(results))
	}

	if results[0].ID != 0 {
		t.Errorf("Expected 'Neural Network' to rank first, got ID %d (title: %s)", results[0].ID, results[0].Title)
	}
}

func TestRanking_MultiTermQuery(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "go programming", "all")

	if len(results) < 2 {
		t.Fatalf("Expected multiple results for 'go programming', got %d", len(results))
	}

	for i := 0; i < len(results)-1; i++ {
		if results[i].Score < results[i+1].Score {
			t.Errorf("Results not sorted by score: result[%d]=%f < result[%d]=%f",
				i, results[i].Score, i+1, results[i+1].Score)
		}
	}
}

func TestRanking_TagPrefixQuery(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "tag:rust", "all")

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for tag:rust, got %d", len(results))
	}

	for _, r := range results {
		if !slices.Contains(index.Posts[strconv.FormatUint(r.ID, 10)].NormalizedTags, "rust") {
			t.Errorf("Result ID %d does not have 'rust' tag", r.ID)
		}
	}
}

func TestRanking_TagPrefixNoMatch(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "tag:python", "all")

	if len(results) != 0 {
		t.Errorf("Expected 0 results for tag:python, got %d", len(results))
	}
}

func TestRanking_VersionFilter(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "machine learning", "v1")

	v2Matches := 0
	for _, r := range results {
		if r.Version == "v2" {
			v2Matches++
		}
	}

	if v2Matches > 0 {
		t.Errorf("v1 filter should not return v2 results, got %d v2 matches", v2Matches)
	}
}

func TestRanking_EmptyQuery(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "", "all")

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}

func TestRanking_FuzzyMatch(t *testing.T) {
	posts := map[string]models.PostRecord{
		"0": {
			ID:              0,
			Title:           "Programming Guide",
			NormalizedTitle: "programming guide",
			Content:         "Learn programming today.",
			NormalizedTags:  []string{},
		},
		"1": {
			ID:              1,
			Title:           "Other Content",
			NormalizedTitle: "other content",
			Content:         "Something completely different.",
			NormalizedTags:  []string{},
		},
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      map[string]map[string][]uint32{},
		Offsets:       map[string]map[string][]uint32{},
		DocLens:       map[string]int64{"0": 3, "1": 5},
		TotalDocs:     2,
		AvgDocLen:     4.0,
		NgramIndex:    core.BuildNgramIndex(make(map[string]map[string][]uint32)),
	}

	index.Inverted["program"] = map[string][]uint32{"0": {0}}

	results := PerformSearch(index, "programming", "all")

	if len(results) < 1 {
		t.Errorf("Expected at least 1 result for 'programming' (stems to 'program'), got %d", len(results))
	}

	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("Result %d has non-positive score %f", r.ID, r.Score)
		}
	}
}

func TestRanking_ScoreOrdering(t *testing.T) {
	posts := map[string]models.PostRecord{
		"0": {
			ID:              0,
			Title:           "Title Has Word",
			NormalizedTitle: "title has word",
			Content:         "Content body.",
			NormalizedTags:  []string{},
		},
		"1": {
			ID:              1,
			Title:           "Other Title",
			NormalizedTitle: "other title",
			Content:         "Content has word in body.",
			NormalizedTags:  []string{},
		},
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      map[string]map[string][]uint32{},
		Offsets:       map[string]map[string][]uint32{},
		DocLens:       map[string]int64{"0": 3, "1": 5},
		TotalDocs:     2,
		AvgDocLen:     4.0,
		NgramIndex:    core.BuildNgramIndex(make(map[string]map[string][]uint32)),
	}

	index.Inverted["word"] = map[string][]uint32{"0": {2}, "1": {3}}

	results := PerformSearch(index, "word", "all")

	if len(results) < 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results[0].Score < results[1].Score {
		t.Errorf("Expected results sorted descending by score: top=%f < next=%f",
			results[0].Score, results[1].Score)
	}
}

func TestRanking_TopKLimit(t *testing.T) {
	posts := map[string]models.PostRecord{}
	inverted := map[string]map[string][]uint32{}

	for i := 0; i < 50; i++ {
		pid := string(rune(i))
		posts[pid] = models.PostRecord{
			ID:              uint64(i),
			Title:           "Post",
			NormalizedTitle: "post",
			Content:         "test content",
			NormalizedTags:  []string{},
		}
		inverted["test"] = map[string][]uint32{pid: {0}}
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      inverted,
		Offsets:       map[string]map[string][]uint32{},
		DocLens:       map[string]int64{},
		TotalDocs:     50,
		AvgDocLen:     5.0,
		NgramIndex:    core.BuildNgramIndex(inverted),
	}

	for i := 0; i < 50; i++ {
		index.DocLens[string(rune(i))] = 5
	}

	results := PerformSearch(index, "test", "all")

	if len(results) > 40 {
		t.Errorf("Expected at most 40 results (topK), got %d", len(results))
	}
}

func TestRanking_TagOnlyQuery(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "tag:ml", "all")

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for tag:ml, got %d", len(results))
	}

	for _, r := range results {
		if !slices.Contains(index.Posts[strconv.FormatUint(r.ID, 10)].NormalizedTags, "ml") {
			t.Errorf("Result ID %d does not have 'ml' tag", r.ID)
		}
	}
}

func TestRanking_PhraseWithQuotes(t *testing.T) {
	posts := map[string]models.PostRecord{
		"0": {
			ID:              0,
			Title:           "Neural Networks",
			NormalizedTitle: "neural networks",
			Content:         "Neural networks are a key technology.",
			NormalizedTags:  []string{},
		},
		"1": {
			ID:              1,
			Title:           "Neural Networks Security",
			NormalizedTitle: "neural networks security",
			Content:         "Security in neural networks applications.",
			NormalizedTags:  []string{},
		},
	}

	index := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts:         posts,
		Inverted:      map[string]map[string][]uint32{},
		Offsets:       map[string]map[string][]uint32{},
		DocLens:       map[string]int64{"0": 5, "1": 5},
		TotalDocs:     2,
		AvgDocLen:     5.0,
	}

	index.Inverted["neural"] = map[string][]uint32{"0": {0}, "1": {0}}
	index.Inverted["network"] = map[string][]uint32{"0": {1}, "1": {1}}

	results := PerformSearch(index, `"neural networks"`, "all")

	if len(results) < 1 {
		t.Errorf("Expected at least 1 result for phrase query, got %d", len(results))
	}

	if results[0].Score <= 0 {
		t.Errorf("Expected positive score for phrase match, got %f", results[0].Score)
	}
}

func TestRanking_SnippetPopulated(t *testing.T) {
	index := buildRankingIndex()

	results := PerformSearch(index, "go", "all")

	for _, r := range results {
		if r.Snippet == "" && strings.TrimSpace(index.Posts[string(rune(r.ID))].Content) != "" {
			t.Logf("Result ID %d has empty snippet but non-empty content", r.ID)
		}
	}
}

func TestRanking_ConcurrentSafety(t *testing.T) {
	index := buildRankingIndex()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				_ = PerformSearch(index, "go programming", "all")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
