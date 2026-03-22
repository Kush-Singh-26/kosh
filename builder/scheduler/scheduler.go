package scheduler

import (
	"context"
	"runtime"

	"golang.org/x/sync/semaphore"
)

// TaskType categorizes SSG operations for weighted scheduling.
type TaskType int

const (
	TaskDefault TaskType = iota
	TaskMarkdown
	TaskImage
	TaskMath
	TaskD2
	TaskSearch
	TaskSocialCard
	TaskAsset
)

// Scheduler weight constants for task resource allocation.
// Higher weights consume more tokens from the semaphore, limiting concurrency for heavy tasks.
const (
	WeightLight    = 50  // Minimal resource usage (markdown parsing)
	WeightModerate = 200 // Moderate resource usage (math rendering, search indexing)
	WeightHeavy    = 400 // Heavy resource usage (image processing, D2 diagrams, esbuild)
	WeightDefault  = 100 // Default weight for unclassified tasks
)

// BuildScheduler coordinates resource usage across multiple concurrent sub-systems.
// It uses weighted semaphore acquisition to prevent resource exhaustion during
// parallel builds with mixed task types.
type BuildScheduler interface {
	Acquire(ctx context.Context, task TaskType) error
	Release(task TaskType)
}

type weightedScheduler struct {
	sem     *semaphore.Weighted
	weights map[TaskType]int64
}

// NewBuildScheduler creates a weighted scheduler with CPU-based token allocation.
// Total tokens = max(CPU cores * 2000, 8000) to allow high concurrency while
// capping peak memory bursts. Weights are tuned for typical SSG workloads.
func NewBuildScheduler() BuildScheduler {
	cpuCount := runtime.NumCPU()
	// Total tokens = Cores * 2000.
	// Large pool to allow high concurrency while still capping peak bursts.
	totalTokens := max(
		// Ensure a minimum floor to prevent deadlocks
		int64(cpuCount*2000), 8000)

	return &weightedScheduler{
		sem: semaphore.NewWeighted(totalTokens),
		weights: map[TaskType]int64{
			TaskDefault:    WeightDefault,
			TaskMarkdown:   WeightLight,    // Very light - text parsing only
			TaskImage:      WeightHeavy,    // Heavy - image decode/encode
			TaskMath:       WeightModerate, // Moderate - JS rendering
			TaskD2:         WeightHeavy,    // Heavy - SVG diagram rendering
			TaskSearch:     WeightModerate, // Moderate - indexing operations
			TaskSocialCard: WeightModerate, // Moderate - image drawing
			TaskAsset:      WeightHeavy,    // Heavy - esbuild bundling
		},
	}
}

func (s *weightedScheduler) Acquire(ctx context.Context, task TaskType) error {
	weight, ok := s.weights[task]
	if !ok {
		weight = s.weights[TaskDefault]
	}
	return s.sem.Acquire(ctx, weight)
}

func (s *weightedScheduler) Release(task TaskType) {
	weight, ok := s.weights[task]
	if !ok {
		weight = s.weights[TaskDefault]
	}
	s.sem.Release(weight)
}
