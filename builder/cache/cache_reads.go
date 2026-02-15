package cache

import (
	"bytes"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func getCachedItem[T any](db *bolt.DB, bucketName string, key []byte) (*T, error) {
	var result *T
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return nil
		}
		data := bucket.Get(key)
		if data == nil {
			return nil
		}

		var item T
		if err := Decode(data, &item); err != nil {
			return err
		}
		result = &item
		return nil
	})
	return result, err
}

// memCacheGet retrieves a PostMeta from the in-memory cache
func (m *Manager) memCacheGet(key string) *PostMeta {
	m.memCacheMu.RLock()
	entry, ok := m.memCache[key]
	m.memCacheMu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		m.memCacheMu.Lock()
		delete(m.memCache, key)
		m.memCacheMu.Unlock()
		return nil
	}

	return entry.meta
}

// memCacheSet stores a PostMeta in the in-memory cache
func (m *Manager) memCacheSet(key string, meta *PostMeta) {
	m.memCacheMu.Lock()
	m.memCache[key] = &memoryCacheEntry{
		meta:      meta,
		expiresAt: time.Now().Add(m.memCacheTTL),
	}
	m.memCacheMu.Unlock()
}

// memCacheDelete removes an entry from the in-memory cache
func (m *Manager) memCacheDelete(key string) {
	m.memCacheMu.Lock()
	delete(m.memCache, key)
	m.memCacheMu.Unlock()
}

// GetPostByPath looks up a post by its file path in a single transaction
func (m *Manager) GetPostByPath(path string) (*PostMeta, error) {
	normalizedPath := utils.NormalizePath(path)

	// Check in-memory cache first
	if cached := m.memCacheGet("path:" + normalizedPath); cached != nil {
		return cached, nil
	}

	var result *PostMeta
	err := m.db.View(func(tx *bolt.Tx) error {
		// First lookup the postID from paths bucket
		paths := tx.Bucket([]byte(BucketPaths))
		if paths == nil {
			return nil
		}
		postID := paths.Get([]byte(normalizedPath))
		if postID == nil {
			return nil
		}

		// Then get the post from posts bucket in the same transaction
		posts := tx.Bucket([]byte(BucketPosts))
		if posts == nil {
			return nil
		}
		data := posts.Get(postID)
		if data == nil {
			return nil
		}

		var meta PostMeta
		if err := Decode(data, &meta); err != nil {
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
func (m *Manager) GetPostByID(postID string) (*PostMeta, error) {
	// Check in-memory cache first
	cacheKey := "id:" + postID
	if cached := m.memCacheGet(cacheKey); cached != nil {
		return cached, nil
	}

	result, err := getCachedItem[PostMeta](m.db, BucketPosts, []byte(postID))
	if err == nil && result != nil {
		m.memCacheSet(cacheKey, result)
	}
	return result, err
}

// GetPostsByIDs retrieves multiple posts by their PostIDs in a single transaction
func (m *Manager) GetPostsByIDs(postIDs []string) (map[string]*PostMeta, error) {
	result := make(map[string]*PostMeta, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))

		for _, id := range postIDs {
			data := postsBucket.Get([]byte(id))
			if data == nil {
				continue
			}

			// Allocate directly on heap to avoid value-to-pointer conversion
			postMeta := new(PostMeta)
			if err := Decode(data, postMeta); err != nil {
				continue
			}
			result[id] = postMeta
		}
		return nil
	})

	return result, err
}

// GetPostsByTemplate retrieves all PostIDs associated with a template
func (m *Manager) GetPostsByTemplate(templatePath string) ([]string, error) {
	var ids []string
	key := []byte(templatePath)

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketDepsTemplates))
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
func (m *Manager) GetSearchRecords(postIDs []string) (map[string]*SearchRecord, error) {
	result := make(map[string]*SearchRecord, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	err := m.db.View(func(tx *bolt.Tx) error {
		searchBucket := tx.Bucket([]byte(BucketSearch))

		for _, id := range postIDs {
			data := searchBucket.Get([]byte(id))
			if data == nil {
				continue
			}

			var record SearchRecord
			if err := Decode(data, &record); err != nil {
				continue
			}
			result[id] = &record
		}
		return nil
	})

	return result, err
}

// GetSearchRecord retrieves the search record for a post
func (m *Manager) GetSearchRecord(postID string) (*SearchRecord, error) {
	return getCachedItem[SearchRecord](m.db, BucketSearch, []byte(postID))
}

// GetSSRArtifact retrieves an SSR artifact
func (m *Manager) GetSSRArtifact(ssrType, inputHash string) (*SSRArtifact, error) {
	key := ssrType + ":" + inputHash
	return getCachedItem[SSRArtifact](m.db, BucketSSR, []byte(key))
}

// GetSSRContent retrieves the actual content for an SSR artifact
func (m *Manager) GetSSRContent(ssrType string, artifact *SSRArtifact) ([]byte, error) {
	category := filepath.Join("ssr", ssrType)
	return m.store.Get(category, artifact.OutputHash, artifact.Compressed)
}

// GetHTMLContent retrieves HTML content for a post
func (m *Manager) GetHTMLContent(post *PostMeta) ([]byte, error) {
	if len(post.InlineHTML) > 0 {
		return post.InlineHTML, nil
	}
	if post.HTMLHash == "" {
		return nil, nil
	}
	return m.store.Get("html", post.HTMLHash, true)
}

// GetPostsByTag returns all PostIDs with a given tag
func (m *Manager) GetPostsByTag(tag string) ([]string, error) {
	prefix := []byte(tag + "/")
	var ids []string

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketTags))
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
		postsBucket := tx.Bucket([]byte(BucketPosts))
		if postsBucket == nil {
			return nil
		}

		c := postsBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var meta PostMeta
			if err := Decode(v, &meta); err != nil {
				continue
			}
			if meta.Version == version {
				result = append(result, PostListMeta{
					Title:   meta.Title,
					Link:    meta.Link,
					Weight:  meta.Weight,
					Version: meta.Version,
					Date:    meta.Date,
				})
			}
		}
		return nil
	})

	return result, err
}

// PostListMeta contains minimal metadata needed for navigation/sorting
type PostListMeta struct {
	Title   string
	Link    string
	Weight  int
	Version string
	Date    time.Time
}
