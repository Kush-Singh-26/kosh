package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// BuildConfig contains all tunable build parameters
// These can be overridden via kosh.build.yaml
type BuildConfig struct {
	// Worker settings
	MaxWorkers     int `yaml:"maxWorkers"`     // Maximum worker pool size (default: 32)
	DefaultWorkers int `yaml:"defaultWorkers"` // Default worker count (default: 12)
	ImageWorkers   int `yaml:"imageWorkers"`   // Parallel image processing workers (default: 24)

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

// DefaultBuildConfig returns the default build configuration
func DefaultBuildConfig() *BuildConfig {
	return &BuildConfig{
		// Workers
		MaxWorkers:     32,
		DefaultWorkers: 12,
		ImageWorkers:   24,

		// Buffers
		MaxBufferSize:       64 * 1024,        // 64KB
		InlineHTMLThreshold: 32 * 1024,        // 32KB
		MaxFileSize:         50 * 1024 * 1024, // 50MB
		FastZstdMax:         64 * 1024,        // 64KB

		// Timeouts
		ShutdownTimeout:  5 * time.Second,
		DebounceDuration: 500 * time.Millisecond,
		TemplateCheckTTL: 2 * time.Second,
		CacheDBTimeout:   10 * time.Second,

		// Search
		MaxSnippetContentLength: 10000,
		DefaultSnippetLength:    150,
		ScoreTitleMatch:         10.0,
		ScoreTagMatch:           5.0,
		ScorePhraseMatch:        15.0,
		ScoreFuzzyModifier:      0.7,
		MaxEditDistance:         2,
		MaxSearchResults:        100,
	}
}

// LoadBuildConfig loads build configuration from kosh.build.yaml
// Returns defaults if file doesn't exist
func LoadBuildConfig() *BuildConfig {
	cfg := DefaultBuildConfig()

	data, err := os.ReadFile("kosh.build.yaml")
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
	if c.MaxWorkers < 1 {
		c.MaxWorkers = 1
	}
	if c.MaxWorkers > 256 {
		c.MaxWorkers = 256
	}
	if c.DefaultWorkers < 1 {
		c.DefaultWorkers = 1
	}
	if c.DefaultWorkers > c.MaxWorkers {
		c.DefaultWorkers = c.MaxWorkers
	}
	if c.ImageWorkers < 1 {
		c.ImageWorkers = 1
	}
	if c.ImageWorkers > 64 {
		c.ImageWorkers = 64
	}

	// Buffers
	if c.MaxBufferSize < 1024 {
		c.MaxBufferSize = 1024 // Minimum 1KB
	}
	if c.MaxBufferSize > 10*1024*1024 {
		c.MaxBufferSize = 10 * 1024 * 1024 // Maximum 10MB
	}
	if c.InlineHTMLThreshold < 1024 {
		c.InlineHTMLThreshold = 1024
	}
	if c.MaxFileSize < 1024*1024 {
		c.MaxFileSize = 1024 * 1024 // Minimum 1MB
	}
	if c.MaxFileSize > 500*1024*1024 {
		c.MaxFileSize = 500 * 1024 * 1024 // Maximum 500MB
	}

	// Timeouts
	if c.ShutdownTimeout < 1*time.Second {
		c.ShutdownTimeout = 1 * time.Second
	}
	if c.ShutdownTimeout > 60*time.Second {
		c.ShutdownTimeout = 60 * time.Second
	}
	if c.DebounceDuration < 10*time.Millisecond {
		c.DebounceDuration = 10 * time.Millisecond
	}
	if c.DebounceDuration > 5*time.Second {
		c.DebounceDuration = 5 * time.Second
	}
	if c.CacheDBTimeout < 1*time.Second {
		c.CacheDBTimeout = 1 * time.Second
	}

	// Search
	if c.DefaultSnippetLength < 50 {
		c.DefaultSnippetLength = 50
	}
	if c.DefaultSnippetLength > 500 {
		c.DefaultSnippetLength = 500
	}
	if c.MaxEditDistance < 0 {
		c.MaxEditDistance = 0
	}
	if c.MaxEditDistance > 4 {
		c.MaxEditDistance = 4
	}
	if c.MaxSearchResults < 10 {
		c.MaxSearchResults = 10
	}
	if c.MaxSearchResults > 1000 {
		c.MaxSearchResults = 1000
	}
}
