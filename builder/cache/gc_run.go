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

	// Step 2: Scan store and find orphaned blobs (parallelized for I/O efficiency)
	type scanResult struct {
		orphaned []struct {
			category string
			hash     string
		}
		scanned int
	}

	resultsCh := make(chan scanResult, 3)
	var scanWg sync.WaitGroup

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

	go func() {
		scanWg.Wait()
		close(resultsCh)
	}()

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
