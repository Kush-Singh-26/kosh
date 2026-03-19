package cache

import (
	"bytes"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
)

func getCachedItem[T any](db *bolt.DB, bucketName string, key []byte) (*T, error) {
	var result *T
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return core.ErrNoContent
		}
		data := bucket.Get(key)
		if data == nil {
			return core.ErrNoContent
		}

		var item T
		if err := core.Decode(data, &item); err != nil {
			return err
		}
		result = &item
		return nil
	})
	return result, err
}

// memCacheGet retrieves a core.PostMeta from the in-memory cache
func (m *Manager) memCacheGet(key string) *core.PostMeta {
	entry, ok := m.memCache.Get(key)
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		m.memCache.Remove(key)
		return nil
	}

	return entry.meta
}

// memCacheSet stores a core.PostMeta in the in-memory cache
func (m *Manager) memCacheSet(key string, meta *core.PostMeta) {
	m.memCache.Add(key, &memoryCacheEntry{
		meta:      meta,
		expiresAt: time.Now().Add(m.memCacheTTL),
	})
}

// memCacheDelete removes an entry from the in-memory cache
func (m *Manager) memCacheDelete(key string) {
	m.memCache.Remove(key)
}

// GetPostByPath looks up a post by its file path in a single transaction
func (m *Manager) GetPostByPath(path string) (*core.PostMeta, error) {
	normalizedPath := fspkg.NormalizePath(path)

	// Check in-memory cache first
	if cached := m.memCacheGet("path:" + normalizedPath); cached != nil {
		return cached, nil
	}

	var result *core.PostMeta
	err := m.db.View(func(tx *bolt.Tx) error {
		// First lookup the postID from paths bucket
		paths := tx.Bucket([]byte(core.BucketPaths))
		if paths == nil {
			return core.ErrNoContent
		}
		postID := paths.Get([]byte(normalizedPath))
		if postID == nil {
			return core.ErrNoContent
		}

		// Then get the post from posts bucket in the same transaction
		posts := tx.Bucket([]byte(core.BucketPosts))
		if posts == nil {
			return core.ErrNoContent
		}
		data := posts.Get(postID)
		if data == nil {
			return core.ErrNoContent
		}

		var meta core.PostMeta
		if err := core.Decode(data, &meta); err != nil {
			return err
		}
		result = &meta
		return nil
	})

	if err == nil && result != nil {
		// Store in memory cache for future lookups
		m.memCacheSet("path:"+normalizedPath, result)
	}

	return result, err
}

// GetPostByID retrieves a post by its PostID
func (m *Manager) GetPostByID(postID string) (*core.PostMeta, error) {
	// Check in-memory cache first
	cacheKey := "id:" + postID
	if cached := m.memCacheGet(cacheKey); cached != nil {
		return cached, nil
	}

	result, err := getCachedItem[core.PostMeta](m.db, core.BucketPosts, []byte(postID))
	if err == nil && result != nil {
		m.memCacheSet(cacheKey, result)
	}
	return result, err
}

// GetPostsByIDs retrieves multiple posts by their PostIDs in a single transaction
// Uses parallel decoding for better performance with large batches
func (m *Manager) GetPostsByIDs(postIDs []string) (map[string]*core.PostMeta, error) {
	result := make(map[string]*core.PostMeta, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	// First, fetch all raw data within a single transaction
	type rawData struct {
		id   string
		data []byte
	}
	rawItems := make([]rawData, 0, len(postIDs))

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))

		for _, id := range postIDs {
			data := postsBucket.Get([]byte(id))
			if data != nil {
				// Copy data out of transaction (BoltDB mmap is valid only during tx)
				copied := make([]byte, len(data))
				copy(copied, data)
				rawItems = append(rawItems, rawData{id: id, data: copied})
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	// core.Decode in parallel for large batches
	if len(rawItems) > 10 {
		var mu sync.Mutex
		var g errgroup.Group
		g.SetLimit(runtime.NumCPU())

		for _, item := range rawItems {
			it := item
			g.Go(func() error {
				postMeta := new(core.PostMeta)
				if err := core.Decode(it.data, postMeta); err == nil {
					mu.Lock()
					result[it.id] = postMeta
					mu.Unlock()
				}
				return nil
			})
		}
		_ = g.Wait()
	} else {
		// Sequential for small batches (avoids goroutine overhead)
		for _, item := range rawItems {
			postMeta := new(core.PostMeta)
			if err := core.Decode(item.data, postMeta); err == nil {
				result[item.id] = postMeta
			}
		}
	}

	return result, nil
}

// GetPostsByTemplate retrieves all PostIDs associated with a template
func (m *Manager) GetPostsByTemplate(templatePath string) ([]string, error) {
	var ids []string
	key := []byte(templatePath)

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketDepsTemplates))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		prefix := append(key, '/')
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})
	return ids, err
}

// GetSearchRecords retrieves multiple search records by PostIDs
func (m *Manager) GetSearchRecords(postIDs []string) (map[string]*core.SearchRecord, error) {
	result := make(map[string]*core.SearchRecord, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	err := m.db.View(func(tx *bolt.Tx) error {
		searchBucket := tx.Bucket([]byte(core.BucketSearch))

		for _, id := range postIDs {
			data := searchBucket.Get([]byte(id))
			if data == nil {
				continue
			}

			var record core.SearchRecord
			if err := core.Decode(data, &record); err != nil {
				continue
			}
			result[id] = &record
		}
		return nil
	})

	return result, err
}

// GetSearchRecord retrieves the search record for a post
func (m *Manager) GetSearchRecord(postID string) (*core.SearchRecord, error) {
	return getCachedItem[core.SearchRecord](m.db, core.BucketSearch, []byte(postID))
}

// GetSSRArtifact retrieves an SSR artifact
func (m *Manager) GetSSRArtifact(ssrType, inputHash string) (*core.SSRArtifact, error) {
	key := ssrType + ":" + inputHash
	return getCachedItem[core.SSRArtifact](m.db, core.BucketSSR, []byte(key))
}

// GetSSRContent retrieves the actual content for an SSR artifact
func (m *Manager) GetSSRContent(ssrType string, artifact *core.SSRArtifact) ([]byte, error) {
	category := filepath.Join("ssr", ssrType)
	return m.store.Get(category, artifact.OutputHash, artifact.Compressed)
}

// GetHTMLContent retrieves HTML content for a post
func (m *Manager) GetHTMLContent(post *core.PostMeta) ([]byte, error) {
	if len(post.InlineHTML) > 0 {
		return post.InlineHTML, nil
	}
	if post.HTMLHash == "" {
		return nil, core.ErrNoContent
	}
	return m.store.Get("html", post.HTMLHash, true)
}

func (m *Manager) GetPostsByTag(tag string) ([]string, error) {
	prefix := []byte(tag + "/")
	var ids []string

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketTags))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})

	return ids, err
}

// GetPostsMetadataByVersion retrieves minimal metadata for posts in a specific version
// This is optimized for ProcessSingle to avoid loading all posts
func (m *Manager) GetPostsMetadataByVersion(version string) ([]PostListMeta, error) {
	var result []PostListMeta

	err := m.db.View(func(tx *bolt.Tx) error {
		versionsBucket := tx.Bucket([]byte(core.BucketVersions))
		if versionsBucket == nil {
			return nil
		}

		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil
		}

		prefix := []byte(version + "/")
		c := versionsBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			postID := k[len(prefix):]

			v := postsBucket.Get(postID)
			if v != nil {
				var meta core.PostMeta
				if err := core.Decode(v, &meta); err == nil {
					result = append(result, PostListMeta{
						Title:   meta.Title,
						Link:    meta.Link,
						Weight:  meta.Weight,
						Version: meta.Version,
						Date:    meta.Date,
					})
				}
			}
		}
		return nil
	})

	return result, err
}
