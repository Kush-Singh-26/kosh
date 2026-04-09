package gc

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	"go.etcd.io/bbolt"
)

const (
	buildsSinceGCSize = 4
	lastGCTimeSize    = 8
)

var ssrTypes = []string{"d2", "math", "math-inline", "math-block", "katex"}

func scanLiveHashes(db *bbolt.DB) (map[string]bool, map[string]bool, error) {
	liveHTMLHashes := make(map[string]bool)
	liveSSRHashes := make(map[string]bool)

	err := db.View(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if err := postsBucket.ForEach(func(_, v []byte) error {
			var post core.PostMeta
			if err := core.Decode(v, &post); err != nil {
				return nil
			}
			if post.HTMLHash != "" {
				liveHTMLHashes[post.HTMLHash] = true
			}
			for _, h := range post.SSRInputHashes {
				liveSSRHashes[h] = true
			}
			return nil
		}); err != nil {
			return err
		}

		ssrBucket := tx.Bucket([]byte(core.BucketSSR))
		return ssrBucket.ForEach(func(_, v []byte) error {
			var artifact core.SSRArtifact
			if err := core.Decode(v, &artifact); err != nil {
				return nil
			}
			liveSSRHashes[artifact.OutputHash] = true
			return nil
		})
	})
	return liveHTMLHashes, liveSSRHashes, err
}

func resolveMaxAge(cfg GCConfig) time.Duration {
	if cfg.MaxAge == 0 {
		return defaultMaxAge
	}
	return cfg.MaxAge
}

func cleanOrphans(s *store.Store, liveHTMLHashes, liveSSRHashes map[string]bool, maxAge time.Duration) (int, int64) {
	deletedTotal := 0
	freedTotal := int64(0)

	deleted, freedBytes, err := s.CleanOrphans("html", liveHTMLHashes, maxAge)
	if err == nil {
		deletedTotal += deleted
		freedTotal += freedBytes
	}

	for _, ssrType := range ssrTypes {
		category := filepath.Join("ssr", ssrType)
		deleted, freedBytes, err := s.CleanOrphans(category, liveSSRHashes, maxAge)
		if err == nil {
			deletedTotal += deleted
			freedTotal += freedBytes
		}
	}

	return deletedTotal, freedTotal
}

func reconcileHTMLRefCounts(refCount *RefCountManager) {
	_ = refCount.ReconcileWithLog(slog.Default())
}

func reconcileSSRRefCounts(db *bbolt.DB, s *store.Store) {
	_ = db.Update(func(tx *bbolt.Tx) error {
		ssrBucket := tx.Bucket([]byte(core.BucketSSR))

		refCounts := make(map[string]int)
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		_ = postsBucket.ForEach(func(_, v []byte) error {
			var post core.PostMeta
			if err := core.Decode(v, &post); err != nil {
				return nil
			}
			for _, h := range post.SSRInputHashes {
				refCounts[h]++
			}
			return nil
		})

		return ssrBucket.ForEach(func(k, v []byte) error {
			var artifact core.SSRArtifact
			if err := core.Decode(v, &artifact); err != nil {
				return nil
			}

			newRefCount := refCounts[artifact.InputHash]
			if artifact.RefCount != newRefCount {
				artifact.RefCount = newRefCount
				if newRefCount == 0 {
					_ = s.Delete(filepath.Join("ssr", artifact.Type), artifact.OutputHash)
					return ssrBucket.Delete(k)
				}
				data, err := core.Encode(&artifact)
				if err != nil {
					return nil
				}
				_ = ssrBucket.Put(k, data)
			}
			return nil
		})
	})
}

func updateGCStats(db *bbolt.DB) {
	_ = db.Update(func(tx *bbolt.Tx) error {
		statsBucket := tx.Bucket([]byte(core.BucketStats))

		countData := make([]byte, buildsSinceGCSize)
		binary.BigEndian.PutUint32(countData, 0)
		_ = statsBucket.Put([]byte("builds_since_gc"), countData)

		gcTime := make([]byte, lastGCTimeSize)
		binary.BigEndian.PutUint64(gcTime, uint64(time.Now().Unix()))
		_ = statsBucket.Put([]byte(core.KeyLastGC), gcTime)

		return nil
	})
}

// RunGC performs garbage collection logic
func RunGC(db *bbolt.DB, s *store.Store, refCount *RefCountManager, cfg GCConfig) (*GCResult, error) {
	start := time.Now()
	result := &GCResult{}

	// Step 1: Collect all live hashes from PostMetas
	liveHTMLHashes, liveSSRHashes, err := scanLiveHashes(db)
	if err != nil {
		return nil, fmt.Errorf("failed to scan live hashes: %w", err)
	}

	result.LiveBlobs = len(liveHTMLHashes) + len(liveSSRHashes)

	// Step 2 & 3: Scan store and find/delete orphaned blobs based on TTL
	if !cfg.DryRun {
		maxAge := resolveMaxAge(cfg)
		deleted, freedBytes := cleanOrphans(s, liveHTMLHashes, liveSSRHashes, maxAge)
		result.DeletedBlobs += deleted
		result.DeletedBytes += freedBytes

		// Set scanned blobs to live + deleted as an approximation
		result.ScannedBlobs = result.LiveBlobs + result.DeletedBlobs
	} else {
		// Dry run is tricky with TTL since we skip the walk in store.go
		// We'll keep the output zeroes and just report what live blobs are
		result.ScannedBlobs = result.LiveBlobs
	}

	// Step 4: Reconcile HTML RefCounts
	if !cfg.DryRun {
		reconcileHTMLRefCounts(refCount)
	}

	// Step 5: Reconcile SSR RefCounts
	if !cfg.DryRun {
		reconcileSSRRefCounts(db, s)
	}

	// Step 5: Update GC stats
	if !cfg.DryRun {
		updateGCStats(db)
	}

	result.Duration = time.Since(start)
	return result, nil
}
