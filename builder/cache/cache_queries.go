package cache

import (
	"encoding/binary"
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

func (m *Manager) ListAllPosts() ([]string, error) {
	var ids []string
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketPosts))
		return bucket.ForEach(func(k, _ []byte) error {
			ids = append(ids, string(k))
			return nil
		})
	})
	return ids, err
}

func (m *Manager) Stats() (*core.CacheStats, error) {
	stats := &core.CacheStats{
		SchemaVersion: core.SchemaVersion,
	}

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		stats.TotalPosts = postsBucket.Stats().KeyN

		ssrBucket := tx.Bucket([]byte(core.BucketSSR))
		stats.TotalSSR = ssrBucket.Stats().KeyN

		statsBucket := tx.Bucket([]byte(core.BucketStats))
		if data := statsBucket.Get([]byte(core.KeyBuildCount)); data != nil {
			stats.BuildCount = int(binary.BigEndian.Uint32(data))
		}
		if data := statsBucket.Get([]byte(core.KeyLastGC)); data != nil {
			stats.LastGC = int64(binary.BigEndian.Uint64(data))
		}

		return postsBucket.ForEach(func(k, v []byte) error {
			var post core.PostMeta
			if err := core.Decode(v, &post); err == nil {
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
		bucket := tx.Bucket([]byte(core.BucketSocialCard))
		if bucket == nil {
			return nil
		}
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
		bucket := tx.Bucket([]byte(core.BucketSocialCard))
		if bucket == nil {
			return nil
		}
		return bucket.Put([]byte(path), []byte(hash))
	})
}

// BatchSetSocialCardHashes stores multiple social card hashes in a single transaction
func (m *Manager) BatchSetSocialCardHashes(hashes map[string]string) error {
	if len(hashes) == 0 {
		return nil
	}
	return m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketSocialCard))
		if bucket == nil {
			return nil
		}
		for path, hash := range hashes {
			if err := bucket.Put([]byte(path), []byte(hash)); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetGraphHash retrieves the graph data hash
func (m *Manager) GetGraphHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		data := meta.Get([]byte(core.KeyGraphHash))
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
		meta := tx.Bucket([]byte(core.BucketMeta))
		return meta.Put([]byte(core.KeyGraphHash), []byte(hash))
	})
}

// GetWasmHash retrieves the stored WASM source hash
func (m *Manager) GetWasmHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		data := meta.Get([]byte(core.KeyWasmHash))
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
		meta := tx.Bucket([]byte(core.BucketMeta))
		return meta.Put([]byte(core.KeyWasmHash), []byte(hash))
	})
}
