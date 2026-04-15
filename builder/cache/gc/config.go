// Package gc provides garbage collection utilities for the cache.
package gc

import (
	"time"
)

const (
	defaultDeadBytesThreshold = 0.30
	defaultMinBuildsBetweenGC = 10
	defaultMaxAge             = 7 * 24 * time.Hour
)

// Config controls garbage collection behavior.
type Config struct {
	DeadBytesThreshold float64       // Trigger GC when dead_bytes / total_bytes > this (default 0.3)
	MinBuildsBetweenGC int           // Minimum builds between automatic GC runs
	MaxAge             time.Duration // Maximum age for unreferenced artifacts (TTL)
	DryRun             bool          // If true, only report what would be deleted
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DeadBytesThreshold: defaultDeadBytesThreshold,
		MinBuildsBetweenGC: defaultMinBuildsBetweenGC,
		MaxAge:             defaultMaxAge, // 7 days TTL for unreferenced artifacts
		DryRun:             false,
	}
}

// Result contains statistics from a GC run.
type Result struct {
	DeletedBlobs int
	DeletedBytes int64
	ScannedBlobs int
	LiveBlobs    int
	Duration     time.Duration
	WasSkipped   bool
	SkipReason   string
}
