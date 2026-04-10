package cache

import (
	"encoding/binary"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"

	"go.etcd.io/bbolt"
)

// ListAllPosts returns all post IDs stored in the cache.
func (manager *Manager) ListAllPosts() ([]string, error) {
	var ids []string
	err := manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketPosts))
		return bucket.ForEach(func(key, _ []byte) error {
			ids = append(ids, string(key))
			return nil
		})
	})
	return ids, err
}

// Stats returns a snapshot of cache statistics.
func (manager *Manager) Stats() (*core.CacheStats, error) {
	stats := &core.CacheStats{
		SchemaVersion: core.SchemaVersion,
	}

	err := manager.db.View(func(tx *bbolt.Tx) error {
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

		return postsBucket.ForEach(func(key, value []byte) error {
			var post core.PostMeta
			if err := core.Decode(value, &post); err == nil {
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

	htmlSize, _ := manager.store.Size("html")
	d2Size, _ := manager.store.Size(filepath.Join("ssr", "d2"))
	katexSize, _ := manager.store.Size(filepath.Join("ssr", "katex"))
	stats.StoreBytes = htmlSize + d2Size + katexSize

	return stats, nil
}

// GetSocialCardHash retrieves the hash for a social card
func (manager *Manager) GetSocialCardHash(path string) (string, error) {
	var hash string
	err := manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketSocialCard))
		if bucket == nil {
			return core.ErrNoContent
		}
		data := bucket.Get([]byte(path))
		if data == nil {
			return core.ErrNoContent
		}
		hash = string(data)
		return nil
	})
	return hash, err
}

// SetSocialCardHash stores the hash for a social card
func (manager *Manager) SetSocialCardHash(path, hash string) error {
	return manager.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketSocialCard))
		if bucket == nil {
			return nil
		}
		return bucket.Put([]byte(path), []byte(hash))
	})
}

// BatchSetSocialCardHashes stores multiple social card hashes in a single transaction
func (manager *Manager) BatchSetSocialCardHashes(hashes map[string]string) error {
	if len(hashes) == 0 {
		return nil
	}
	return manager.db.Update(func(tx *bbolt.Tx) error {
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
func (manager *Manager) GetGraphHash() (string, error) {
	var hash string
	err := manager.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(core.BucketMeta))
		data := metaBucket.Get([]byte(core.KeyGraphHash))
		if data == nil {
			return core.ErrNoContent
		}
		hash = string(data)
		return nil
	})
	return hash, err
}

// SetGraphHash stores the graph data hash
func (manager *Manager) SetGraphHash(hash string) error {
	return manager.db.Update(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(core.BucketMeta))
		return metaBucket.Put([]byte(core.KeyGraphHash), []byte(hash))
	})
}

// GetWasmHash retrieves the stored WASM source hash
func (manager *Manager) GetWasmHash() (string, error) {
	var hash string
	err := manager.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(core.BucketMeta))
		data := metaBucket.Get([]byte(core.KeyWasmHash))
		if data == nil {
			return core.ErrNoContent
		}
		hash = string(data)
		return nil
	})
	return hash, err
}

// SetWasmHash stores the WASM source hash
func (manager *Manager) SetWasmHash(hash string) error {
	return manager.db.Update(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(core.BucketMeta))
		return metaBucket.Put([]byte(core.KeyWasmHash), []byte(hash))
	})
}
