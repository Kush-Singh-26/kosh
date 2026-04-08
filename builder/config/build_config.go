package config

import (
	"time"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
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

const (
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
func (c *BuildConfig) validate() {
	// Workers
	if c.MaxWorkers < MinWorkers {
		c.MaxWorkers = MinWorkers
	}
	if c.MaxWorkers > MaxWorkers {
		c.MaxWorkers = MaxWorkers
	}
	if c.DefaultWorkers < MinWorkers {
		c.DefaultWorkers = MinWorkers
	}
	if c.DefaultWorkers > c.MaxWorkers {
		c.DefaultWorkers = c.MaxWorkers
	}
	// Buffers
	if c.MaxBufferSize < MinBufferSize {
		c.MaxBufferSize = MinBufferSize // Minimum 1KB
	}
	if c.MaxBufferSize > MaxBufferSize {
		c.MaxBufferSize = MaxBufferSize // Maximum 10MB
	}
	if c.InlineHTMLThreshold < MinInlineHTMLThreshold {
		c.InlineHTMLThreshold = MinInlineHTMLThreshold
	}
	if c.MaxFileSize < MinMaxFileSize {
		c.MaxFileSize = MinMaxFileSize // Minimum 1MB
	}
	if c.MaxFileSize > MaxMaxFileSize {
		c.MaxFileSize = MaxMaxFileSize // Maximum 500MB
	}

	// Timeouts
	if c.ShutdownTimeout < MinShutdownTimeout {
		c.ShutdownTimeout = MinShutdownTimeout
	}
	if c.ShutdownTimeout > MaxShutdownTimeout {
		c.ShutdownTimeout = MaxShutdownTimeout
	}
	if c.DebounceDuration < MinDebounceDuration {
		c.DebounceDuration = MinDebounceDuration
	}
	if c.DebounceDuration > MaxDebounceDuration {
		c.DebounceDuration = MaxDebounceDuration
	}
	if c.CacheDBTimeout < MinCacheDBTimeout {
		c.CacheDBTimeout = MinCacheDBTimeout
	}

	// Search
	if c.DefaultSnippetLength < MinSnippetLength {
		c.DefaultSnippetLength = MinSnippetLength
	}
	if c.DefaultSnippetLength > MaxSnippetLength {
		c.DefaultSnippetLength = MaxSnippetLength
	}
	if c.MaxEditDistance < MinEditDistance {
		c.MaxEditDistance = MinEditDistance
	}
	if c.MaxEditDistance > MaxEditDistance {
		c.MaxEditDistance = MaxEditDistance
	}
	if c.MaxSearchResults < MinSearchResults {
		c.MaxSearchResults = MinSearchResults
	}
	if c.MaxSearchResults > MaxSearchResults {
		c.MaxSearchResults = MaxSearchResults
	}
}
