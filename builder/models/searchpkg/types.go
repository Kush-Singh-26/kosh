// Package searchpkg contains the models and MessagePack methods for the Kosh search engine.
package searchpkg

// SearchRankingConfig defines BM25 and custom scoring weights.
type SearchRankingConfig struct {
	TitleBoost       float64 `yaml:"titleBoost"`
	TagBoost         float64 `yaml:"tagBoost"`
	DescriptionBoost float64 `yaml:"descriptionBoost"`
	BM25K1           float64 `yaml:"bm25K1"`
	BM25B            float64 `yaml:"bm25b"`
}
