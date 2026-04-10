package gc

import (
	"encoding/binary"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"go.etcd.io/bbolt"
)

const (
	refCountBytes = 4
	truncHashLen  = 16
)

// RefCountManager manages reference counts for cached blobs.
type RefCountManager struct {
	db *bbolt.DB
}

// NewRefCountManager creates a RefCountManager for the provided DB.
func NewRefCountManager(db *bbolt.DB) *RefCountManager {
	return &RefCountManager{db: db}
}

// Decrement decreases the refcount for a hash.
func (manager *RefCountManager) Decrement(hash string) (uint32, error) {
	if hash == "" {
		return 0, nil
	}
	var newCount uint32
	err := manager.db.Update(func(tx *bbolt.Tx) error {
		return manager.DecrementTx(tx, hash, &newCount)
	})
	return newCount, err
}

// DecrementTx decreases the refcount using an existing transaction.
func (manager *RefCountManager) DecrementTx(tx *bbolt.Tx, hash string, newCountOut *uint32) error {
	bucket := tx.Bucket([]byte(core.BucketRefCount))
	if bucket == nil {
		return nil
	}
	if value := bucket.Get([]byte(hash)); value != nil {
		count := binary.BigEndian.Uint32(value)
		if count > 0 {
			count--
		}
		if newCountOut != nil {
			*newCountOut = count
		}
		data := make([]byte, refCountBytes)
		binary.BigEndian.PutUint32(data, count)
		return bucket.Put([]byte(hash), data)
	}
	return nil
}

// Increment increases the refcount for a hash.
func (manager *RefCountManager) Increment(hash string) error {
	if hash == "" {
		return nil
	}
	return manager.db.Update(func(tx *bbolt.Tx) error {
		return manager.IncrementTx(tx, hash)
	})
}

// IncrementTx increases the refcount using an existing transaction.
func (manager *RefCountManager) IncrementTx(tx *bbolt.Tx, hash string) error {
	bucket := tx.Bucket([]byte(core.BucketRefCount))
	if bucket == nil {
		return nil
	}
	var count uint32
	if value := bucket.Get([]byte(hash)); value != nil {
		count = binary.BigEndian.Uint32(value)
	}
	count++
	data := make([]byte, refCountBytes)
	binary.BigEndian.PutUint32(data, count)
	return bucket.Put([]byte(hash), data)
}

// Get returns the current refcount for a hash.
func (manager *RefCountManager) Get(hash string) uint32 {
	if hash == "" {
		return 0
	}
	var count uint32
	_ = manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketRefCount))
		if bucket == nil {
			return nil
		}
		if value := bucket.Get([]byte(hash)); value != nil {
			count = binary.BigEndian.Uint32(value)
		}
		return nil
	})
	return count
}

// Reconcile recomputes refcounts without logging.
func (manager *RefCountManager) Reconcile() error {
	return manager.ReconcileWithLog(nil)
}

// ReconcileWithLog recomputes all refcounts from the posts bucket and logs discrepancies.
func (manager *RefCountManager) ReconcileWithLog(logger *slog.Logger) error {
	return manager.db.Update(func(tx *bbolt.Tx) error {
		refBucket := tx.Bucket([]byte(core.BucketRefCount))
		if refBucket == nil {
			return nil
		}

		// Capture old counts before clearing
		oldCounts := make(map[string]uint32)
		_ = refBucket.ForEach(func(key, value []byte) error {
			oldCounts[string(key)] = binary.BigEndian.Uint32(value)
			return nil
		})

		// Clear all refcounts
		_ = refBucket.ForEach(func(key, _ []byte) error {
			return refBucket.Delete(key)
		})

		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil
		}

		// Recompute from posts
		counts := make(map[string]uint32)
		_ = postsBucket.ForEach(func(_, value []byte) error {
			var post core.PostMeta
			if err := core.Decode(value, &post); err != nil {
				return nil
			}
			if post.HTMLHash != "" {
				counts[post.HTMLHash]++
			}
			return nil
		})

		// Write new counts and log discrepancies
		for hash, count := range counts {
			data := make([]byte, refCountBytes)
			binary.BigEndian.PutUint32(data, count)
			_ = refBucket.Put([]byte(hash), data)

			if logger != nil {
				if oldCount, existed := oldCounts[hash]; existed && oldCount != count {
					truncatedHash := hash
					if len(truncatedHash) > truncHashLen {
						truncatedHash = truncatedHash[:truncHashLen] + "..."
					}
					logger.Warn("refcount mismatch",
						"hash", truncatedHash,
						"stored", oldCount,
						"computed", count,
						"delta", int(count)-int(oldCount))
				}
			}
		}

		// Log orphaned refcounts (hashes with refcount but no posts)
		if logger != nil {
			for hash, oldCount := range oldCounts {
				if _, exists := counts[hash]; !exists && oldCount > 0 {
					truncatedHash := hash
					if len(truncatedHash) > truncHashLen {
						truncatedHash = truncatedHash[:truncHashLen] + "..."
					}
					logger.Warn("orphaned refcount (no posts reference this hash)",
						"hash", truncatedHash,
						"stored_count", oldCount)
				}
			}
		}

		return nil
	})
}
