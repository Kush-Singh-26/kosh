package utils

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
	TaskSearch
	TaskSocialCard
)

// BuildScheduler coordinates resource usage across multiple concurrent sub-systems.
type BuildScheduler interface {
	Acquire(ctx context.Context, task TaskType) error
	Release(task TaskType)
}

type weightedScheduler struct {
	sem     *semaphore.Weighted
	weights map[TaskType]int64
}

func NewBuildScheduler() BuildScheduler {
	cpuCount := runtime.NumCPU()
	// Total tokens = Cores * 1000.
	// Large pool to allow high concurrency while still capping peak bursts.
	totalTokens := int64(cpuCount * 1000)

	// Ensure a minimum floor to prevent deadlocks
	if totalTokens < 4000 {
		totalTokens = 4000
	}

	return &weightedScheduler{
		sem: semaphore.NewWeighted(totalTokens),
		weights: map[TaskType]int64{
			TaskDefault:    100,
			TaskMarkdown:   100, // Very light
			TaskImage:      500, // Heavy
			TaskMath:       250, // Moderate (JS)
			TaskSearch:     250, // Moderate
			TaskSocialCard: 250, // Moderate (Drawing)
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

// GlobalScheduler is the singleton used across the build process
var GlobalScheduler = NewBuildScheduler()
