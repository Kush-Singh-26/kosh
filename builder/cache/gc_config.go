package cache

import (
	"encoding/binary"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
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

// ShouldRunGC checks if GC should run based on conditions
func (m *Manager) ShouldRunGC(cfg GCConfig) (bool, string) {
	var buildsSinceGC int
	_ = m.db.View(func(tx *bolt.Tx) error {
		statsBucket := tx.Bucket([]byte(BucketStats))
		if data := statsBucket.Get([]byte("builds_since_gc")); data != nil {
			buildsSinceGC = int(binary.BigEndian.Uint32(data))
		}
		return nil
	})

	if buildsSinceGC < cfg.MinBuildsBetweenGC {
		return false, fmt.Sprintf("only %d builds since last GC (min: %d)", buildsSinceGC, cfg.MinBuildsBetweenGC)
	}

	return false, "no GC trigger conditions met"
}
