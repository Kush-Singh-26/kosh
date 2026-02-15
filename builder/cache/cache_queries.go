package cache

import (
	"encoding/binary"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

// ListAllPosts returns all PostIDs
func (m *Manager) ListAllPosts() ([]string, error) {
	var ids []string
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketPosts))
		return bucket.ForEach(func(k, _ []byte) error {
			ids = append(ids, string(k))
			return nil
		})
	})
	return ids, err
}

// Stats returns current cache statistics
func (m *Manager) Stats() (*CacheStats, error) {
	stats := &CacheStats{
		SchemaVersion: SchemaVersion,
	}

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		stats.TotalPosts = postsBucket.Stats().KeyN

		ssrBucket := tx.Bucket([]byte(BucketSSR))
		stats.TotalSSR = ssrBucket.Stats().KeyN

		statsBucket := tx.Bucket([]byte(BucketStats))
		if data := statsBucket.Get([]byte(KeyBuildCount)); data != nil {
			stats.BuildCount = int(binary.BigEndian.Uint32(data))
		}
		if data := statsBucket.Get([]byte(KeyLastGC)); data != nil {
			stats.LastGC = int64(binary.BigEndian.Uint64(data))
		}

		return postsBucket.ForEach(func(k, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err == nil {
				if len(post.InlineHTML) > 0 {
					stats.InlinePosts++
				} else if post.HTMLHash != "" {
					stats.HashedPosts++
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	htmlSize, _ := m.store.Size("html")
	d2Size, _ := m.store.Size(filepath.Join("ssr", "d2"))
	katexSize, _ := m.store.Size(filepath.Join("ssr", "katex"))
	stats.StoreBytes = htmlSize + d2Size + katexSize

	// Runtime metrics are no longer tracked in Manager struct
	// but kept in API for compatibility
	return stats, nil
}

// GetSocialCardHash retrieves the hash for a social card
func (m *Manager) GetSocialCardHash(path string) (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSocialCard))
		data := bucket.Get([]byte(path))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetSocialCardHash stores the hash for a social card
func (m *Manager) SetSocialCardHash(path, hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSocialCard))
		return bucket.Put([]byte(path), []byte(hash))
	})
}

// GetGraphHash retrieves the graph data hash
func (m *Manager) GetGraphHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		data := meta.Get([]byte(KeyGraphHash))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetGraphHash stores the graph data hash
func (m *Manager) SetGraphHash(hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		return meta.Put([]byte(KeyGraphHash), []byte(hash))
	})
}

// GetWasmHash retrieves the stored WASM source hash
func (m *Manager) GetWasmHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		data := meta.Get([]byte(KeyWasmHash))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetWasmHash stores the WASM source hash
func (m *Manager) SetWasmHash(hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		return meta.Put([]byte(KeyWasmHash), []byte(hash))
	})
}
