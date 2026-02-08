package cache

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	stats, err := m.Stats()
	if err != nil {
		return false, "failed to get stats"
	}

	// Check builds since last GC
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

	// Check dead bytes ratio
	if stats.StoreBytes > 0 && stats.DeadBytes > 0 {
		ratio := float64(stats.DeadBytes) / float64(stats.StoreBytes)
		if ratio > cfg.DeadBytesThreshold {
			return true, fmt.Sprintf("dead bytes ratio %.2f%% exceeds threshold %.2f%%", ratio*100, cfg.DeadBytesThreshold*100)
		}
	}

	return false, "no GC trigger conditions met"
}

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
				return nil // Skip corrupt entries
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

	// Step 2: Scan store and find orphaned blobs (parallelized for I/O efficiency)
	type scanResult struct {
		orphaned []struct {
			category string
			hash     string
		}
		scanned int
	}

	resultsCh := make(chan scanResult, 3) // html, d2, katex
	var scanWg sync.WaitGroup

	// Scan HTML store concurrently
	scanWg.Add(1)
	go func() {
		defer scanWg.Done()
		htmlHashes, err := m.store.ListHashes("html")
		if err != nil {
			return
		}
		res := scanResult{orphaned: make([]struct {
			category string
			hash     string
		}, 0, len(htmlHashes))}
		for _, hash := range htmlHashes {
			res.scanned++
			if !liveHTMLHashes[hash] {
				res.orphaned = append(res.orphaned, struct {
					category string
					hash     string
				}{"html", hash})
			}
		}
		resultsCh <- res
	}()

	// Scan SSR stores concurrently
	for _, ssrType := range []string{"d2", "katex"} {
		scanWg.Add(1)
		go func(ssrType string) {
			defer scanWg.Done()
			category := filepath.Join("ssr", ssrType)
			hashes, err := m.store.ListHashes(category)
			if err != nil {
				return
			}
			res := scanResult{orphaned: make([]struct {
				category string
				hash     string
			}, 0, len(hashes))}
			for _, hash := range hashes {
				res.scanned++
				if !liveSSRHashes[hash] {
					res.orphaned = append(res.orphaned, struct {
						category string
						hash     string
					}{category, hash})
				}
			}
			resultsCh <- res
		}(ssrType)
	}

	// Close channel when all goroutines complete
	go func() {
		scanWg.Wait()
		close(resultsCh)
	}()

	// Collect results
	var orphanedBlobs []struct {
		category string
		hash     string
	}
	for res := range resultsCh {
		result.ScannedBlobs += res.scanned
		orphanedBlobs = append(orphanedBlobs, res.orphaned...)
	}

	// Step 3: Delete orphaned blobs (unless dry run)
	if !cfg.DryRun {
		for _, blob := range orphanedBlobs {
			// Get size before deleting for stats
			rawPath := filepath.Join(m.basePath, "store", blob.category, blob.hash[0:2], blob.hash[2:4], blob.hash+".raw")
			zstPath := filepath.Join(m.basePath, "store", blob.category, blob.hash[0:2], blob.hash[2:4], blob.hash+".zst")

			if info, err := os.Stat(rawPath); err == nil {
				result.DeletedBytes += info.Size()
			}
			if info, err := os.Stat(zstPath); err == nil {
				result.DeletedBytes += info.Size()
			}

			if err := m.store.Delete(blob.category, blob.hash); err == nil {
				result.DeletedBlobs++
			}
		}
	} else {
		result.DeletedBlobs = len(orphanedBlobs)
	}

	// Step 4: Reconcile SSR RefCounts
	if !cfg.DryRun {
		_ = m.db.Update(func(tx *bolt.Tx) error {
			ssrBucket := tx.Bucket([]byte(BucketSSR))

			// First pass: reset all ref counts
			refCounts := make(map[string]int)

			// Count references from posts
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

			// Update artifacts with correct ref counts
			return ssrBucket.ForEach(func(k, v []byte) error {
				var artifact SSRArtifact
				if err := Decode(v, &artifact); err != nil {
					return nil
				}

				newRefCount := refCounts[artifact.InputHash]
				if artifact.RefCount != newRefCount {
					artifact.RefCount = newRefCount
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

			// Reset builds since GC
			countData := make([]byte, 4)
			binary.BigEndian.PutUint32(countData, 0)
			_ = statsBucket.Put([]byte("builds_since_gc"), countData)

			// Update last GC time
			gcTime := make([]byte, 8)
			binary.BigEndian.PutUint64(gcTime, uint64(time.Now().Unix()))
			_ = statsBucket.Put([]byte(KeyLastGC), gcTime)

			return nil
		})
	}

	result.Duration = time.Since(start)
	return result, nil
}

// Verify checks cache integrity
func (m *Manager) Verify() ([]string, error) {
	var errors []string

	// Check all posts have valid path mappings
	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))

		return postsBucket.ForEach(func(k, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt post data: %s", string(k)))
				return nil
			}

			// Check path mapping
			normalizedPath := normalizePath(post.Path)
			mappedID := pathsBucket.Get([]byte(normalizedPath))
			if mappedID == nil {
				errors = append(errors, fmt.Sprintf("missing path mapping: %s -> %s", normalizedPath, post.PostID))
			} else if string(mappedID) != post.PostID {
				errors = append(errors, fmt.Sprintf("path mapping mismatch: %s -> %s (expected %s)", normalizedPath, string(mappedID), post.PostID))
			}

			// Check HTML exists in store
			if post.HTMLHash != "" && !m.store.Exists("html", post.HTMLHash) {
				errors = append(errors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.PostID))
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Check SSR artifacts have valid blobs
	err = m.db.View(func(tx *bolt.Tx) error {
		ssrBucket := tx.Bucket([]byte(BucketSSR))
		return ssrBucket.ForEach(func(k, v []byte) error {
			var artifact SSRArtifact
			if err := Decode(v, &artifact); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt SSR artifact: %s", string(k)))
				return nil
			}

			category := filepath.Join("ssr", artifact.Type)
			if !m.store.Exists(category, artifact.OutputHash) {
				errors = append(errors, fmt.Sprintf("missing SSR blob: %s for %s", artifact.OutputHash, string(k)))
			}

			return nil
		})
	})

	return errors, err
}

// Clear removes all cache data
func (m *Manager) Clear() error {
	// Close the database first
	_ = m.db.Close()

	// Remove all files
	_ = os.RemoveAll(m.basePath)

	// Recreate
	newManager, err := Open(m.basePath, false)
	if err != nil {
		return err
	}

	m.db = newManager.db
	m.store = newManager.store
	m.dirty = make(map[string]bool)

	return nil
}

// Rebuild triggers a full cache rebuild by clearing the cache
func (m *Manager) Rebuild() error {
	return m.Clear()
}

// IncrementBuildCount increments the build counter
func (m *Manager) IncrementBuildCount() error {
	return m.db.Update(func(tx *bolt.Tx) error {
		statsBucket := tx.Bucket([]byte(BucketStats))

		// Increment total build count
		buildCount := uint32(1)
		if data := statsBucket.Get([]byte(KeyBuildCount)); data != nil {
			buildCount = binary.BigEndian.Uint32(data) + 1
		}
		countData := make([]byte, 4)
		binary.BigEndian.PutUint32(countData, buildCount)
		if err := statsBucket.Put([]byte(KeyBuildCount), countData); err != nil {
			return err
		}

		// Increment builds since GC
		buildsSinceGC := uint32(1)
		if data := statsBucket.Get([]byte("builds_since_gc")); data != nil {
			buildsSinceGC = binary.BigEndian.Uint32(data) + 1
		}
		sinceGCData := make([]byte, 4)
		binary.BigEndian.PutUint32(sinceGCData, buildsSinceGC)
		return statsBucket.Put([]byte("builds_since_gc"), sinceGCData)
	})
}
