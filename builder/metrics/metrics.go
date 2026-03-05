package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type BuildMetrics struct {
	StartTime       time.Time
	EndTime         time.Time
	PostsProcessed  atomic.Int64 // atomic — safe for concurrent goroutine access
	CacheHits       atomic.Int64 // atomic — safe for concurrent goroutine access
	CacheMisses     atomic.Int64 // atomic — safe for concurrent goroutine access
	PanicsRecovered int32        // atomic — safe for concurrent goroutine access
}

func NewBuildMetrics() *BuildMetrics {
	return &BuildMetrics{
		StartTime: time.Now(),
	}
}

func (m *BuildMetrics) RecordEnd() {
	m.EndTime = time.Now()
}

func (m *BuildMetrics) TotalDuration() time.Duration {
	if m.EndTime.IsZero() {
		return time.Since(m.StartTime)
	}
	return m.EndTime.Sub(m.StartTime)
}

func (m *BuildMetrics) IncrementPostsProcessed() {
	m.PostsProcessed.Add(1)
}

func (m *BuildMetrics) IncrementCacheHit() {
	m.CacheHits.Add(1)
}

func (m *BuildMetrics) IncrementCacheMiss() {
	m.CacheMisses.Add(1)
}

func (m *BuildMetrics) IncrementPanicsRecovered() {
	atomic.AddInt32(&m.PanicsRecovered, 1)
}

func (m *BuildMetrics) String() string {
	duration := m.TotalDuration()
	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	result := fmt.Sprintf("📊 Built %d posts in %v (cache: %d/%d hits, %.0f%%)\n",
		m.PostsProcessed.Load(),
		duration,
		hits,
		total,
		hitRate,
	)

	panics := atomic.LoadInt32(&m.PanicsRecovered)
	if panics > 0 {
		result += fmt.Sprintf("⚠️  %d panic(s) recovered during build\n", panics)
	}

	return result
}

func (m *BuildMetrics) Print() {
	fmt.Println(m.String())
}
