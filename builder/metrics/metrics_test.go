package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestNewBuildMetrics(t *testing.T) {
	m := NewBuildMetrics()

	if m.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}

	if m.EndTime.IsZero() == false {
		t.Error("EndTime should be zero initially")
	}

	if m.PostsProcessed != 0 {
		t.Errorf("PostsProcessed should be 0, got %d", m.PostsProcessed)
	}

	if m.CacheHits != 0 {
		t.Errorf("CacheHits should be 0, got %d", m.CacheHits)
	}

	if m.CacheMisses != 0 {
		t.Errorf("CacheMisses should be 0, got %d", m.CacheMisses)
	}
}

func TestRecordEnd(t *testing.T) {
	m := NewBuildMetrics()
	before := time.Now()
	m.RecordEnd()
	after := time.Now()

	if m.EndTime.Before(before) || m.EndTime.After(after) {
		t.Error("EndTime should be set to current time")
	}
}

func TestTotalDuration(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*BuildMetrics)
		expected func(time.Duration) bool
	}{
		{
			name: "returns elapsed time when end not set",
			setup: func(m *BuildMetrics) {
				m.StartTime = time.Now().Add(-time.Second)
			},
			expected: func(d time.Duration) bool {
				return d >= time.Second
			},
		},
		{
			name: "returns total duration when end is set",
			setup: func(m *BuildMetrics) {
				m.StartTime = time.Now().Add(-5 * time.Second)
				m.EndTime = time.Now()
			},
			expected: func(d time.Duration) bool {
				return d >= 5*time.Second && d < 6*time.Second
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewBuildMetrics()
			tt.setup(m)
			duration := m.TotalDuration()
			if !tt.expected(duration) {
				t.Errorf("TotalDuration() = %v, unexpected value", duration)
			}
		})
	}
}

func TestIncrementPostsProcessed(t *testing.T) {
	m := NewBuildMetrics()

	m.IncrementPostsProcessed()
	if m.PostsProcessed != 1 {
		t.Errorf("PostsProcessed = %d, want 1", m.PostsProcessed)
	}

	m.IncrementPostsProcessed()
	m.IncrementPostsProcessed()
	if m.PostsProcessed != 3 {
		t.Errorf("PostsProcessed = %d, want 3", m.PostsProcessed)
	}
}

func TestIncrementCacheHit(t *testing.T) {
	m := NewBuildMetrics()

	m.IncrementCacheHit()
	if m.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", m.CacheHits)
	}

	m.IncrementCacheHit()
	m.IncrementCacheHit()
	if m.CacheHits != 3 {
		t.Errorf("CacheHits = %d, want 3", m.CacheHits)
	}
}

func TestIncrementCacheMiss(t *testing.T) {
	m := NewBuildMetrics()

	m.IncrementCacheMiss()
	if m.CacheMisses != 1 {
		t.Errorf("CacheMisses = %d, want 1", m.CacheMisses)
	}

	m.IncrementCacheMiss()
	m.IncrementCacheMiss()
	if m.CacheMisses != 3 {
		t.Errorf("CacheMisses = %d, want 3", m.CacheMisses)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*BuildMetrics)
		contains []string
	}{
		{
			name: "empty build",
			setup: func(m *BuildMetrics) {
				m.PostsProcessed = 0
				m.CacheHits = 0
				m.CacheMisses = 0
			},
			contains: []string{"Built 0 posts", "cache: 0/0 hits", "0%"},
		},
		{
			name: "build with posts and cache hits",
			setup: func(m *BuildMetrics) {
				m.PostsProcessed = 10
				m.CacheHits = 8
				m.CacheMisses = 2
			},
			contains: []string{"Built 10 posts", "cache: 8/10 hits", "80%"},
		},
		{
			name: "all cache misses",
			setup: func(m *BuildMetrics) {
				m.PostsProcessed = 5
				m.CacheHits = 0
				m.CacheMisses = 5
			},
			contains: []string{"Built 5 posts", "cache: 0/5 hits", "0%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewBuildMetrics()
			tt.setup(m)

			result := m.String()
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("String() = %q, should contain %q", result, expected)
				}
			}
		})
	}
}

func TestStringFormat(t *testing.T) {
	m := NewBuildMetrics()
	m.PostsProcessed = 27
	m.CacheHits = 23
	m.CacheMisses = 4
	m.StartTime = time.Now().Add(-time.Second)

	result := m.String()

	if !strings.HasPrefix(result, "📊 Built") {
		t.Error("String() should start with emoji and 'Built'")
	}

	if !strings.Contains(result, "posts in") {
		t.Error("String() should contain 'posts in'")
	}

	if !strings.Contains(result, "(cache:") {
		t.Error("String() should contain cache info in parentheses")
	}

	if !strings.Contains(result, "hits,") {
		t.Error("String() should contain 'hits,'")
	}

	if !strings.HasSuffix(result, "%)\n") {
		t.Error("String() should end with '%)")
	}
}
