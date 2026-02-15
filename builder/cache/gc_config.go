package cache

import (
	"time"
)

// GCConfig controls garbage collection behavior
type GCConfig struct {
	DeadBytesThreshold float64 // Trigger GC when dead_bytes / total_bytes > this (default 0.3)
	MinBuildsBetweenGC int     // Minimum builds between automatic GC runs
	DryRun             bool    // If true, only report what would be deleted
}

// DefaultGCConfig returns sensible defaults
func DefaultGCConfig() GCConfig {
	return GCConfig{
		DeadBytesThreshold: 0.30,
		MinBuildsBetweenGC: 10,
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
