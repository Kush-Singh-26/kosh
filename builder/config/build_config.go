package config

import (
	"time"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// Build configuration constants.
const (
	// DefaultMaxWorkers is the maximum number of concurrent workers for build operations.
	DefaultMaxWorkers     = 32
	DefaultDefaultWorkers = 12

	DefaultMaxBufferSize       = 64 * 1024
	DefaultInlineHTMLThreshold = 32 * 1024
	DefaultMaxFileSize         = 50 * 1024 * 1024
	DefaultFastZstdMax         = 64 * 1024

	DefaultShutdownTimeout  = 5 * time.Second
	DefaultDebounceDuration = 500 * time.Millisecond
	DefaultTemplateCheckTTL = 2 * time.Second
	DefaultCacheDBTimeout   = 10 * time.Second

	DefaultMaxSnippetContentLength = 10000
	DefaultSnippetLength           = 150
	DefaultScoreTitleMatch         = 10.0
	DefaultScoreTagMatch           = 5.0
	DefaultScorePhraseMatch        = 15.0
	DefaultScoreFuzzyModifier      = 0.7
	DefaultMaxEditDistance         = 2
	DefaultMaxSearchResults        = 100
)

// Worker configuration limits.
const (
	// MinWorkers is the minimum allowed number of workers.
	MinWorkers = 1
	MaxWorkers = 256

	MinBufferSize = 1024
	MaxBufferSize = 10 * 1024 * 1024

	MinInlineHTMLThreshold = 1024

	MinMaxFileSize = 1024 * 1024
	MaxMaxFileSize = 500 * 1024 * 1024

	MinShutdownTimeout = 1 * time.Second
	MaxShutdownTimeout = 60 * time.Second

	MinDebounceDuration = 10 * time.Millisecond
	MaxDebounceDuration = 5 * time.Second

	MinCacheDBTimeout = 1 * time.Second

	MinSnippetLength = 50
	MaxSnippetLength = 500

	MinEditDistance = 0
	MaxEditDistance = 4

	MinSearchResults = 10
	MaxSearchResults = 1000
)

// BuildConfig contains all tunable build parameters
// These can be overridden via kosh.build.yaml
type BuildConfig struct {
	// Debug settings
	Debug bool `yaml:"debug"` // Enable debug output (default: false)

	// Worker settings
	MaxWorkers     int `yaml:"maxWorkers"`     // Maximum worker pool size (default: 32)
	DefaultWorkers int `yaml:"defaultWorkers"` // Default worker count (default: 12)
	// Buffer/Cache settings
	MaxBufferSize       int `yaml:"maxBufferSize"`       // Max buffer size for pools (default: 64KB)
	InlineHTMLThreshold int `yaml:"inlineHTMLThreshold"` // Size threshold for inline HTML storage (default: 32KB)
	MaxFileSize         int `yaml:"maxFileSize"`         // Max file size to load in memory (default: 50MB)
	FastZstdMax         int `yaml:"fastZstdMax"`         // Threshold for fast zstd compression (default: 64KB)

	// Timeouts
	ShutdownTimeout  time.Duration `yaml:"shutdownTimeout"`  // Server shutdown timeout (default: 5s)
	DebounceDuration time.Duration `yaml:"debounceDuration"` // File watcher debounce (default: 500ms)
	TemplateCheckTTL time.Duration `yaml:"templateCheckTTL"` // Template mtime check TTL (default: 2s)
	CacheDBTimeout   time.Duration `yaml:"cacheDBTimeout"`   // BoltDB timeout (default: 10s)

	// Search settings
	MaxSnippetContentLength int     `yaml:"maxSnippetContentLength"` // Max content length for snippets (default: 10000)
	DefaultSnippetLength    int     `yaml:"defaultSnippetLength"`    // Default snippet length (default: 150)
	ScoreTitleMatch         float64 `yaml:"scoreTitleMatch"`         // BM25 title match score (default: 10.0)
	ScoreTagMatch           float64 `yaml:"scoreTagMatch"`           // BM25 tag match score (default: 5.0)
	ScorePhraseMatch        float64 `yaml:"scorePhraseMatch"`        // BM25 phrase match score (default: 15.0)
	ScoreFuzzyModifier      float64 `yaml:"scoreFuzzyModifier"`      // Fuzzy match score modifier (default: 0.7)
	MaxEditDistance         int     `yaml:"maxEditDistance"`         // Max fuzzy edit distance (default: 2)
	MaxSearchResults        int     `yaml:"maxSearchResults"`        // Max search results (default: 100)
}

// DefaultBuildConfig returns the default build configuration.
func DefaultBuildConfig() *BuildConfig {
	return &BuildConfig{
		// Workers
		MaxWorkers:     DefaultMaxWorkers,
		DefaultWorkers: DefaultDefaultWorkers,
		// Buffers
		MaxBufferSize:       DefaultMaxBufferSize,       // 64KB
		InlineHTMLThreshold: DefaultInlineHTMLThreshold, // 32KB
		MaxFileSize:         DefaultMaxFileSize,         // 50MB
		FastZstdMax:         DefaultFastZstdMax,         // 64KB

		// Timeouts
		ShutdownTimeout:  DefaultShutdownTimeout,
		DebounceDuration: DefaultDebounceDuration,
		TemplateCheckTTL: DefaultTemplateCheckTTL,
		CacheDBTimeout:   DefaultCacheDBTimeout,

		// Search
		MaxSnippetContentLength: DefaultMaxSnippetContentLength,
		DefaultSnippetLength:    DefaultSnippetLength,
		ScoreTitleMatch:         DefaultScoreTitleMatch,
		ScoreTagMatch:           DefaultScoreTagMatch,
		ScorePhraseMatch:        DefaultScorePhraseMatch,
		ScoreFuzzyModifier:      DefaultScoreFuzzyModifier,
		MaxEditDistance:         DefaultMaxEditDistance,
		MaxSearchResults:        DefaultMaxSearchResults,
	}
}

// LoadBuildConfig loads build configuration from kosh.build.yaml
// Returns defaults if file doesn't exist
func LoadBuildConfig() *BuildConfig {
	return LoadBuildConfigFs(afero.NewOsFs())
}

// LoadBuildConfigFs loads build configuration from kosh.build.yaml using the provided filesystem
func LoadBuildConfigFs(fs afero.Fs) *BuildConfig {
	cfg := DefaultBuildConfig()

	data, err := afero.ReadFile(fs, "kosh.build.yaml")
	if err != nil {
		// File doesn't exist, use defaults
		return cfg
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		// Parse error, use defaults
		return cfg
	}

	// Validate and clamp values
	cfg.validate()

	return cfg
}

// validate ensures configuration values are within reasonable bounds
func (buildCfg *BuildConfig) validate() {
	// Workers
	if buildCfg.MaxWorkers < MinWorkers {
		buildCfg.MaxWorkers = MinWorkers
	}
	if buildCfg.MaxWorkers > MaxWorkers {
		buildCfg.MaxWorkers = MaxWorkers
	}
	if buildCfg.DefaultWorkers < MinWorkers {
		buildCfg.DefaultWorkers = MinWorkers
	}
	if buildCfg.DefaultWorkers > buildCfg.MaxWorkers {
		buildCfg.DefaultWorkers = buildCfg.MaxWorkers
	}
	// Buffers
	if buildCfg.MaxBufferSize < MinBufferSize {
		buildCfg.MaxBufferSize = MinBufferSize // Minimum 1KB
	}
	if buildCfg.MaxBufferSize > MaxBufferSize {
		buildCfg.MaxBufferSize = MaxBufferSize // Maximum 10MB
	}
	if buildCfg.InlineHTMLThreshold < MinInlineHTMLThreshold {
		buildCfg.InlineHTMLThreshold = MinInlineHTMLThreshold
	}
	if buildCfg.MaxFileSize < MinMaxFileSize {
		buildCfg.MaxFileSize = MinMaxFileSize // Minimum 1MB
	}
	if buildCfg.MaxFileSize > MaxMaxFileSize {
		buildCfg.MaxFileSize = MaxMaxFileSize // Maximum 500MB
	}

	// Timeouts
	if buildCfg.ShutdownTimeout < MinShutdownTimeout {
		buildCfg.ShutdownTimeout = MinShutdownTimeout
	}
	if buildCfg.ShutdownTimeout > MaxShutdownTimeout {
		buildCfg.ShutdownTimeout = MaxShutdownTimeout
	}
	if buildCfg.DebounceDuration < MinDebounceDuration {
		buildCfg.DebounceDuration = MinDebounceDuration
	}
	if buildCfg.DebounceDuration > MaxDebounceDuration {
		buildCfg.DebounceDuration = MaxDebounceDuration
	}
	if buildCfg.CacheDBTimeout < MinCacheDBTimeout {
		buildCfg.CacheDBTimeout = MinCacheDBTimeout
	}

	// Search
	if buildCfg.DefaultSnippetLength < MinSnippetLength {
		buildCfg.DefaultSnippetLength = MinSnippetLength
	}
	if buildCfg.DefaultSnippetLength > MaxSnippetLength {
		buildCfg.DefaultSnippetLength = MaxSnippetLength
	}
	if buildCfg.MaxEditDistance < MinEditDistance {
		buildCfg.MaxEditDistance = MinEditDistance
	}
	if buildCfg.MaxEditDistance > MaxEditDistance {
		buildCfg.MaxEditDistance = MaxEditDistance
	}
	if buildCfg.MaxSearchResults < MinSearchResults {
		buildCfg.MaxSearchResults = MinSearchResults
	}
	if buildCfg.MaxSearchResults > MaxSearchResults {
		buildCfg.MaxSearchResults = MaxSearchResults
	}
}
