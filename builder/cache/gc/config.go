package gc

import (
	"time"
)

const (
	defaultDeadBytesThreshold = 0.30
	defaultMinBuildsBetweenGC = 10
	defaultMaxAge             = 7 * 24 * time.Hour
)

// GCConfig controls garbage collection behavior
type GCConfig struct {
	DeadBytesThreshold float64       // Trigger GC when dead_bytes / total_bytes > this (default 0.3)
	MinBuildsBetweenGC int           // Minimum builds between automatic GC runs
	MaxAge             time.Duration // Maximum age for unreferenced artifacts (TTL)
	DryRun             bool          // If true, only report what would be deleted
}

// DefaultGCConfig returns sensible defaults
func DefaultGCConfig() GCConfig {
	return GCConfig{
		DeadBytesThreshold: defaultDeadBytesThreshold,
		MinBuildsBetweenGC: defaultMinBuildsBetweenGC,
		MaxAge:             defaultMaxAge, // 7 days TTL for unreferenced artifacts
		DryRun:             false,
	}
}

// GCResult contains statistics from a GC run
type GCResult struct {
	DeletedBlobs int
	DeletedBytes int64
	ScannedBlobs int
	LiveBlobs    int
	Duration     time.Duration
	WasSkipped   bool
	SkipReason   string
}
