package search

import (
	"fmt"
	"maps"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

func TestFilterScorer(t *testing.T) {
	idx := buildRankingIndex()
	for _, tc := range []struct {
		name  string
		terms []core.QueryTerm
		want  map[uint32]float64
	}{
		{"plain", []core.QueryTerm{{Term: "go"}, {Term: "missing"}}, map[uint32]float64{0: 3, 1: 7}},
		{"required", []core.QueryTerm{{Term: "go", Required: true}}, map[uint32]float64{0: 3}},
		{"excluded", []core.QueryTerm{{Term: "go", Excluded: true}}, map[uint32]float64{1: 7}},
		{"missing required", []core.QueryTerm{{Term: "missing", Required: true}}, map[uint32]float64{}},
		{"missing excluded", []core.QueryTerm{{Term: "missing", Excluded: true}}, map[uint32]float64{0: 3, 1: 7}},
		{"combined", []core.QueryTerm{{Term: "program", Required: true}, {Term: "rust", Excluded: true}}, map[uint32]float64{0: 3}},
		{"conflicting", []core.QueryTerm{{Term: "go", Required: true, Excluded: true}}, map[uint32]float64{}},
		{"duplicate", []core.QueryTerm{{Term: "go", Required: true}, {Term: "go", Required: true}}, map[uint32]float64{0: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &ScoringOptions{Scores: map[uint32]float64{0: 3, 1: 7}, TermInfos: tc.terms}
			(&FilterScorer{}).Score(&Context{Index: idx}, opts)
			if !maps.Equal(opts.Scores, tc.want) {
				t.Fatalf("scores = %v, want %v", opts.Scores, tc.want)
			}
		})
	}
}

func BenchmarkFilterScorer(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		for _, required := range []bool{false, true} {
			b.Run(fmt.Sprintf("documents=%d/required=%t", count, required), func(b *testing.B) {
				// Reuse one sorted posting list to isolate filtering from index construction.
				idx := buildRankingIndex()
				idx.Terms = []string{"common"}
				idx.DocIDs = make([]uint32, count)
				for i := 1; i < count; i++ {
					idx.DocIDs[i] = 1
				}
				idx.PostingOffsets = []uint32{0, uint32(count)}
				idx.PosOffsets = []uint32{0, 0}
				opts := &ScoringOptions{
					Scores:    make(map[uint32]float64, count),
					TermInfos: []core.QueryTerm{{Term: "common", Required: required}},
				}
				for i := range count {
					opts.Scores[uint32(i)] = 1
				}
				ctx := &Context{Index: idx}
				scorer := &FilterScorer{}
				b.ReportAllocs()
				for b.Loop() {
					scorer.Score(ctx, opts)
				}
			})
		}
	}
}
