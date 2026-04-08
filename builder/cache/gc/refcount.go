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
func (m *RefCountManager) Decrement(hash string) (uint32, error) {
	if hash == "" {
		return 0, nil
	}
	var newCount uint32
	err := m.db.Update(func(tx *bbolt.Tx) error {
		return m.DecrementTx(tx, hash, &newCount)
	})
	return newCount, err
}

// DecrementTx decreases the refcount using an existing transaction.
func (m *RefCountManager) DecrementTx(tx *bbolt.Tx, hash string, newCountOut *uint32) error {
	bucket := tx.Bucket([]byte(core.BucketRefCount))
	if bucket == nil {
		return nil
	}
	if v := bucket.Get([]byte(hash)); v != nil {
		count := binary.BigEndian.Uint32(v)
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
func (m *RefCountManager) Increment(hash string) error {
	if hash == "" {
		return nil
	}
	return m.db.Update(func(tx *bbolt.Tx) error {
		return m.IncrementTx(tx, hash)
	})
}

// IncrementTx increases the refcount using an existing transaction.
func (m *RefCountManager) IncrementTx(tx *bbolt.Tx, hash string) error {
	bucket := tx.Bucket([]byte(core.BucketRefCount))
	if bucket == nil {
		return nil
	}
	var count uint32
	if v := bucket.Get([]byte(hash)); v != nil {
		count = binary.BigEndian.Uint32(v)
	}
	count++
	data := make([]byte, refCountBytes)
	binary.BigEndian.PutUint32(data, count)
	return bucket.Put([]byte(hash), data)
}

// Get returns the current refcount for a hash.
func (m *RefCountManager) Get(hash string) uint32 {
	if hash == "" {
		return 0
	}
	var count uint32
	_ = m.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketRefCount))
		if bucket == nil {
			return nil
		}
		if v := bucket.Get([]byte(hash)); v != nil {
			count = binary.BigEndian.Uint32(v)
		}
		return nil
	})
	return count
}

// Reconcile recomputes refcounts without logging.
func (m *RefCountManager) Reconcile() error {
	return m.ReconcileWithLog(nil)
}

// ReconcileWithLog recomputes all refcounts from the posts bucket and logs discrepancies.
func (m *RefCountManager) ReconcileWithLog(logger *slog.Logger) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		refBucket := tx.Bucket([]byte(core.BucketRefCount))
		if refBucket == nil {
			return nil
		}

		// Capture old counts before clearing
		oldCounts := make(map[string]uint32)
		_ = refBucket.ForEach(func(k, v []byte) error {
			oldCounts[string(k)] = binary.BigEndian.Uint32(v)
			return nil
		})

		// Clear all refcounts
		_ = refBucket.ForEach(func(k, _ []byte) error {
			return refBucket.Delete(k)
		})

		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil
		}

		// Recompute from posts
		counts := make(map[string]uint32)
		_ = postsBucket.ForEach(func(_, v []byte) error {
			var post core.PostMeta
			if err := core.Decode(v, &post); err != nil {
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
				if old, existed := oldCounts[hash]; existed && old != count {
					truncHash := hash
					if len(truncHash) > truncHashLen {
						truncHash = truncHash[:truncHashLen] + "..."
					}
					logger.Warn("refcount mismatch",
						"hash", truncHash,
						"stored", old,
						"computed", count,
						"delta", int(count)-int(old))
				}
			}
		}

		// Log orphaned refcounts (hashes with refcount but no posts)
		if logger != nil {
			for hash, old := range oldCounts {
				if _, exists := counts[hash]; !exists && old > 0 {
					truncHash := hash
					if len(truncHash) > truncHashLen {
						truncHash = truncHash[:truncHashLen] + "..."
					}
					logger.Warn("orphaned refcount (no posts reference this hash)",
						"hash", truncHash,
						"stored_count", old)
				}
			}
		}

		return nil
	})
}
