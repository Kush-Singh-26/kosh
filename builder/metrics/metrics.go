package metrics

import (
	"fmt"
	"time"
)

type BuildMetrics struct {
	StartTime      time.Time
	EndTime        time.Time
	PostsProcessed int
	CacheHits      int
	CacheMisses    int
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
	m.PostsProcessed++
}

func (m *BuildMetrics) IncrementCacheHit() {
	m.CacheHits++
}

func (m *BuildMetrics) IncrementCacheMiss() {
	m.CacheMisses++
}

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

func (m *BuildMetrics) Print() {
	fmt.Println(m.String())
}
