package cache

import (
	"encoding/binary"

	bolt "go.etcd.io/bbolt"
)

type RefCountManager struct {
	db *bolt.DB
}

func newRefCountManager(db *bolt.DB) *RefCountManager {
	return &RefCountManager{db: db}
}

func (m *RefCountManager) Increment(hash string) error {
	if hash == "" {
		return nil
	}
	return m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketRefCount))
		if bucket == nil {
			return nil
		}
		var count uint32
		if v := bucket.Get([]byte(hash)); v != nil {
			count = binary.BigEndian.Uint32(v)
		}
		count++
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, count)
		return bucket.Put([]byte(hash), data)
	})
}

func (m *RefCountManager) Decrement(hash string) (uint32, error) {
	if hash == "" {
		return 0, nil
	}
	var newCount uint32
	err := m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketRefCount))
		if bucket == nil {
			return nil
		}
		if v := bucket.Get([]byte(hash)); v != nil {
			newCount = binary.BigEndian.Uint32(v)
			if newCount > 0 {
				newCount--
			}
			data := make([]byte, 4)
			binary.BigEndian.PutUint32(data, newCount)
			return bucket.Put([]byte(hash), data)
		}
		return nil
	})
	return newCount, err
}

func (m *RefCountManager) Get(hash string) uint32 {
	if hash == "" {
		return 0
	}
	var count uint32
	_ = m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketRefCount))
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

func (m *RefCountManager) Reconcile() error {
	return m.db.Update(func(tx *bolt.Tx) error {
		refBucket := tx.Bucket([]byte(BucketRefCount))
		if refBucket == nil {
			return nil
		}

		_ = refBucket.ForEach(func(k, _ []byte) error {
			return refBucket.Delete(k)
		})

		postsBucket := tx.Bucket([]byte(BucketPosts))
		if postsBucket == nil {
			return nil
		}

		counts := make(map[string]uint32)
		_ = postsBucket.ForEach(func(_, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err != nil {
				return nil
			}
			if post.HTMLHash != "" {
				counts[post.HTMLHash]++
			}
			return nil
		})

		for hash, count := range counts {
			data := make([]byte, 4)
			binary.BigEndian.PutUint32(data, count)
			_ = refBucket.Put([]byte(hash), data)
		}

		return nil
	})
}
