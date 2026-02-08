// Package metrics provides build performance tracking and telemetry.
package metrics

import (
	"fmt"
	"time"
)

// BuildMetrics tracks performance data during the build process.
type BuildMetrics struct {
	// Timing
	StartTime        time.Time
	EndTime          time.Time
	PostProcessTime  time.Duration
	RenderTime       time.Duration
	AssetProcessTime time.Duration
	CacheLoadTime    time.Duration
	CacheSaveTime    time.Duration

	// Counters
	PostsProcessed   int
	CacheHits        int
	CacheMisses      int
	FilesWritten     int
	FilesSkipped     int
	ImagesProcessed  int
	DiagramsRendered int

	// Memory (optional, for profiling builds)
	PeakMemoryMB float64

	// Incremental build info
	IsIncremental bool
	ChangedFiles  []string
}

// NewBuildMetrics creates a new metrics instance.
func NewBuildMetrics() *BuildMetrics {
	return &BuildMetrics{
		StartTime: time.Now(),
	}
}

// RecordStart marks the start of a build phase.
func (m *BuildMetrics) RecordStart() {
	m.StartTime = time.Now()
}

// RecordEnd marks the end of the build and calculates totals.
func (m *BuildMetrics) RecordEnd() {
	m.EndTime = time.Now()
}

// TotalDuration returns the total build duration.
func (m *BuildMetrics) TotalDuration() time.Duration {
	if m.EndTime.IsZero() {
		return time.Since(m.StartTime)
	}
	return m.EndTime.Sub(m.StartTime)
}

// CacheHitRate returns the cache hit percentage.
func (m *BuildMetrics) CacheHitRate() float64 {
	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(m.CacheHits) / float64(total) * 100
}

// IncrementPostsProcessed increments the posts counter.
func (m *BuildMetrics) IncrementPostsProcessed() {
	m.PostsProcessed++
}

// IncrementCacheHit increments the cache hit counter.
func (m *BuildMetrics) IncrementCacheHit() {
	m.CacheHits++
}

// IncrementCacheMiss increments the cache miss counter.
func (m *BuildMetrics) IncrementCacheMiss() {
	m.CacheMisses++
}

// String returns a formatted summary of the build metrics (minimal single-line format).
func (m *BuildMetrics) String() string {
	duration := m.TotalDuration()
	total := m.CacheHits + m.CacheMisses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(m.CacheHits) / float64(total) * 100
	}

	return fmt.Sprintf("📊 Built %d posts in %v (cache: %d/%d hits, %.0f%%)\n",
		m.PostsProcessed,
		duration,
		m.CacheHits,
		total,
		hitRate,
	)
}

// Print outputs the metrics to stdout.
func (m *BuildMetrics) Print() {
	fmt.Println(m.String())
}
