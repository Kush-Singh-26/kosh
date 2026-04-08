package metrics

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

const (
	percentScale = 100
	bytesPerKiB  = 1024
)

// BuildMetrics records build performance metrics.
type BuildMetrics struct {
	StartTime       time.Time
	EndTime         time.Time
	PostsProcessed  atomic.Int64 // atomic — safe for concurrent goroutine access
	CacheHits       atomic.Int64 // atomic — safe for concurrent goroutine access
	CacheMisses     atomic.Int64 // atomic — safe for concurrent goroutine access
	PanicsRecovered int32        // atomic — safe for concurrent goroutine access

	// Image optimization metrics
	OriginalImageSize  atomic.Int64
	OptimizedImageSize atomic.Int64
	ImagesOptimized    atomic.Int64
	ImageResizeSkipped atomic.Int64
	AssetsProcessed    atomic.Int64
	SVGsMinified       atomic.Int64
}

// NewBuildMetrics creates a new BuildMetrics instance.
func NewBuildMetrics() *BuildMetrics {
	return &BuildMetrics{
		StartTime: time.Now(),
	}
}

// Reset clears all metrics counters.
func (m *BuildMetrics) Reset() {
	m.StartTime = time.Now()
	m.EndTime = time.Time{}
	m.PostsProcessed.Store(0)
	m.CacheHits.Store(0)
	m.CacheMisses.Store(0)
	atomic.StoreInt32(&m.PanicsRecovered, 0)
	m.OriginalImageSize.Store(0)
	m.OptimizedImageSize.Store(0)
	m.ImagesOptimized.Store(0)
	m.ImageResizeSkipped.Store(0)
	m.AssetsProcessed.Store(0)
	m.SVGsMinified.Store(0)
}

// RecordEnd sets the end time for the build.
func (m *BuildMetrics) RecordEnd() {
	m.EndTime = time.Now()
}

// TotalDuration returns the elapsed build duration.
func (m *BuildMetrics) TotalDuration() time.Duration {
	if m.EndTime.IsZero() {
		return time.Since(m.StartTime)
	}
	return m.EndTime.Sub(m.StartTime)
}

// IncrementPostsProcessed increments the posts processed counter.
func (m *BuildMetrics) IncrementPostsProcessed() {
	m.PostsProcessed.Add(1)
}

// IncrementCacheHit increments the cache hit counter.
func (m *BuildMetrics) IncrementCacheHit() {
	m.CacheHits.Add(1)
}

// IncrementCacheMiss increments the cache miss counter.
func (m *BuildMetrics) IncrementCacheMiss() {
	m.CacheMisses.Add(1)
}

// IncrementPanicsRecovered increments the recovered panics counter.
func (m *BuildMetrics) IncrementPanicsRecovered() {
	atomic.AddInt32(&m.PanicsRecovered, 1)
}

// RecordImageOptimization records the size delta for optimized images.
func (m *BuildMetrics) RecordImageOptimization(original, optimized int64) {
	m.OriginalImageSize.Add(original)
	m.OptimizedImageSize.Add(optimized)
	m.ImagesOptimized.Add(1)
}

// RecordImageResizeSkipped increments the resize skipped counter.
func (m *BuildMetrics) RecordImageResizeSkipped() {
	m.ImageResizeSkipped.Add(1)
}

// IncrementAssetsProcessed increments the assets processed counter.
func (m *BuildMetrics) IncrementAssetsProcessed() {
	m.AssetsProcessed.Add(1)
}

// IncrementSVGsMinified increments the SVG minified counter.
func (m *BuildMetrics) IncrementSVGsMinified() {
	m.SVGsMinified.Add(1)
}

// String renders a human-readable summary of the metrics.
func (m *BuildMetrics) String() string {
	duration := m.TotalDuration()
	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * percentScale
	}

	result := fmt.Sprintf("Built %d posts in %v (cache: %d/%d hits, %.0f%%)\n",
		m.PostsProcessed.Load(),
		duration,
		hits,
		total,
		hitRate,
	)

	if assets := m.AssetsProcessed.Load(); assets > 0 {
		minified := m.SVGsMinified.Load()
		if minified > 0 {
			result += fmt.Sprintf("Processed %d assets (%d SVGs minified)\n", assets, minified)
		} else {
			result += fmt.Sprintf("Processed %d assets\n", assets)
		}
	}

	images := m.ImagesOptimized.Load()
	if images > 0 {
		orig := m.OriginalImageSize.Load()
		opt := m.OptimizedImageSize.Load()
		saved := orig - opt
		savingsPercent := float64(0)
		if orig > 0 {
			savingsPercent = float64(saved) / float64(orig) * percentScale
		}
		result += fmt.Sprintf("Optimized %d images (saved %s, %.1f%%)\n",
			images,
			formatBytes(saved),
			savingsPercent,
		)
		if skipped := m.ImageResizeSkipped.Load(); skipped > 0 {
			result += fmt.Sprintf("Skipped resize for %d small images\n", skipped)
		}
	}

	panics := atomic.LoadInt32(&m.PanicsRecovered)
	if panics > 0 {
		result += fmt.Sprintf("%d panic(s) recovered during build\n", panics)
	}

	return result
}

func formatBytes(b int64) string {
	const unit = bytesPerKiB
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Print logs the metrics summary.
func (m *BuildMetrics) Print() {
	slog.Info(m.String())
}
