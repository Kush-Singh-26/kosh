package cache

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// RunGC performs garbage collection
func (m *Manager) RunGC(cfg GCConfig) (*GCResult, error) {
	start := time.Now()
	result := &GCResult{}

	// Step 1: Collect all live hashes from PostMetas
	liveHTMLHashes := make(map[string]bool)
	liveSSRHashes := make(map[string]bool)

	err := m.db.View(func(tx *bolt.Tx) error {
		// Scan posts for HTML hashes
		postsBucket := tx.Bucket([]byte(BucketPosts))
		err := postsBucket.ForEach(func(_, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err != nil {
				return nil
			}
			if post.HTMLHash != "" {
				liveHTMLHashes[post.HTMLHash] = true
			}
			for _, h := range post.SSRInputHashes {
				liveSSRHashes[h] = true
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Scan SSR artifacts for output hashes
		ssrBucket := tx.Bucket([]byte(BucketSSR))
		return ssrBucket.ForEach(func(k, v []byte) error {
			var artifact SSRArtifact
			if err := Decode(v, &artifact); err != nil {
				return nil
			}
			liveSSRHashes[artifact.OutputHash] = true
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan live hashes: %w", err)
	}

	result.LiveBlobs = len(liveHTMLHashes) + len(liveSSRHashes)

	// Step 2 & 3: Scan store and find/delete orphaned blobs based on TTL
	if !cfg.DryRun {
		// Set a default maxAge if not provided (e.g., 7 days)
		maxAge := cfg.MaxAge
		if maxAge == 0 {
			maxAge = 7 * 24 * time.Hour
		}

		// Clean HTML artifacts
		deleted, freedBytes, err := m.store.CleanOrphans("html", liveHTMLHashes, maxAge)
		if err == nil {
			result.DeletedBlobs += deleted
			result.DeletedBytes += freedBytes
		}

		// Clean SSR artifacts
		for _, ssrType := range []string{"d2", "katex"} {
			category := filepath.Join("ssr", ssrType)
			deleted, freedBytes, err := m.store.CleanOrphans(category, liveSSRHashes, maxAge)
			if err == nil {
				result.DeletedBlobs += deleted
				result.DeletedBytes += freedBytes
			}
		}

		// Set scanned blobs to live + deleted as an approximation
		result.ScannedBlobs = result.LiveBlobs + result.DeletedBlobs
	} else {
		// Dry run is tricky with TTL since we skip the walk in store.go
		// We'll keep the output zeroes and just report what live blobs are
		result.ScannedBlobs = result.LiveBlobs
	}

	// Step 4: Reconcile HTML RefCounts
	if !cfg.DryRun {
		_ = m.refCount.ReconcileWithLog(slog.Default())
	}

	// Step 5: Reconcile SSR RefCounts
	if !cfg.DryRun {
		_ = m.db.Update(func(tx *bolt.Tx) error {
			ssrBucket := tx.Bucket([]byte(BucketSSR))

			refCounts := make(map[string]int)

			postsBucket := tx.Bucket([]byte(BucketPosts))
			_ = postsBucket.ForEach(func(_, v []byte) error {
				var post PostMeta
				if err := Decode(v, &post); err != nil {
					return nil
				}
				for _, h := range post.SSRInputHashes {
					refCounts[h]++
				}
				return nil
			})

			return ssrBucket.ForEach(func(k, v []byte) error {
				var artifact SSRArtifact
				if err := Decode(v, &artifact); err != nil {
					return nil
				}

				newRefCount := refCounts[artifact.InputHash]
				if artifact.RefCount != newRefCount {
					artifact.RefCount = newRefCount
					if newRefCount == 0 {
						// Safe to delete from store if refcount is 0
						_ = m.store.Delete(filepath.Join("ssr", artifact.Type), artifact.OutputHash)
						return ssrBucket.Delete(k)
					}
					data, err := Encode(&artifact)
					if err != nil {
						return nil
					}
					_ = ssrBucket.Put(k, data)
				}
				return nil
			})
		})
	}

	// Step 5: Update GC stats
	if !cfg.DryRun {
		_ = m.db.Update(func(tx *bolt.Tx) error {
			statsBucket := tx.Bucket([]byte(BucketStats))

			countData := make([]byte, 4)
			binary.BigEndian.PutUint32(countData, 0)
			_ = statsBucket.Put([]byte("builds_since_gc"), countData)

			gcTime := make([]byte, 8)
			binary.BigEndian.PutUint64(gcTime, uint64(time.Now().Unix()))
			_ = statsBucket.Put([]byte(KeyLastGC), gcTime)

			return nil
		})
	}

	result.Duration = time.Since(start)
	return result, nil
}
