package cache

import (
	"bytes"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
)

const parallelDecodeThreshold = 10

func getCachedItem[T any](db *bbolt.DB, bucketName string, key []byte) (*T, error) {
	var result *T
	err := db.View(func(tx *bbolt.Tx) error {
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
func (manager *Manager) memCacheGet(key string) *core.PostMeta {
	entry, ok := manager.memCache.Get(key)
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		manager.memCache.Remove(key)
		return nil
	}

	return entry.meta
}

// memCacheSet stores a core.PostMeta in the in-memory cache
func (manager *Manager) memCacheSet(key string, meta *core.PostMeta) {
	manager.memCache.Add(key, &memoryCacheEntry{
		meta:      meta,
		expiresAt: time.Now().Add(manager.memCacheTTL),
	})
}

// memCacheDelete removes an entry from the in-memory cache
func (manager *Manager) memCacheDelete(key string) {
	manager.memCache.Remove(key)
}

// GetPostByPath looks up a post by its file path in a single transaction
func (manager *Manager) GetPostByPath(path string) (*core.PostMeta, error) {
	normalizedPath := fspkg.NormalizePath(path)

	// Check in-memory cache first
	if cached := manager.memCacheGet("path:" + normalizedPath); cached != nil {
		return cached, nil
	}

	var result *core.PostMeta
	err := manager.db.View(func(tx *bbolt.Tx) error {
		// First lookup the postID from paths bucket
		pathsBucket := tx.Bucket([]byte(core.BucketPaths))
		if pathsBucket == nil {
			return core.ErrNoContent
		}
		postID := pathsBucket.Get([]byte(normalizedPath))
		if postID == nil {
			return core.ErrNoContent
		}

		// Then get the post from posts bucket in the same transaction
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return core.ErrNoContent
		}
		data := postsBucket.Get(postID)
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
		manager.memCacheSet("path:"+normalizedPath, result)
	}

	return result, err
}

// GetPostByID retrieves a post by its PostID
func (manager *Manager) GetPostByID(postID string) (*core.PostMeta, error) {
	// Check in-memory cache first
	cacheKey := "id:" + postID
	if cached := manager.memCacheGet(cacheKey); cached != nil {
		return cached, nil
	}

	result, err := getCachedItem[core.PostMeta](manager.db, core.BucketPosts, []byte(postID))
	if err == nil && result != nil {
		manager.memCacheSet(cacheKey, result)
	}
	return result, err
}

// GetPostsByIDs retrieves multiple posts by their PostIDs in a single transaction
// Uses parallel decoding for better performance with large batches
func (manager *Manager) GetPostsByIDs(postIDs []string) (map[string]*core.PostMeta, error) {
	result := make(map[string]*core.PostMeta, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	rawItems, err := manager.fetchRawItems(postIDs)
	if err != nil {
		return result, err
	}

	// core.Decode in parallel for large batches
	if len(rawItems) > parallelDecodeThreshold {
		var mutex sync.Mutex
		var errorGroup errgroup.Group
		errorGroup.SetLimit(runtime.NumCPU())

		for _, item := range rawItems {
			currentItem := item
			errorGroup.Go(func() error {
				postMeta := new(core.PostMeta)
				if err := core.Decode(currentItem.data, postMeta); err != nil {
					return err
				}
				mutex.Lock()
				result[currentItem.id] = postMeta
				mutex.Unlock()
				return nil
			})
		}
		if err := errorGroup.Wait(); err != nil {
			return nil, err
		}
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
func (manager *Manager) GetPostsByTemplate(templatePath string) ([]string, error) {
	var ids []string
	key := []byte(templatePath)

	err := manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketDepsTemplates))
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		prefix := append(key, '/')
		for k, _ := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cursor.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})
	return ids, err
}

// GetSearchRecords retrieves multiple search records by PostIDs
func (manager *Manager) GetSearchRecords(postIDs []string) (map[string]*core.SearchRecord, error) {
	result := make(map[string]*core.SearchRecord, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	err := manager.db.View(func(tx *bbolt.Tx) error {
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
func (manager *Manager) GetSearchRecord(postID string) (*core.SearchRecord, error) {
	return getCachedItem[core.SearchRecord](manager.db, core.BucketSearch, []byte(postID))
}

// GetSSRArtifact retrieves an SSR artifact
func (manager *Manager) GetSSRArtifact(ssrType, inputHash string) (*core.SSRArtifact, error) {
	key := ssrType + ":" + inputHash
	return getCachedItem[core.SSRArtifact](manager.db, core.BucketSSR, []byte(key))
}

// GetFragment retrieves a pre-rendered UI fragment from the cache.
func (manager *Manager) GetFragment(key string) (string, error) {
	var result string
	err := manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketFragments))
		if bucket == nil {
			return core.ErrNoContent
		}
		data := bucket.Get([]byte(key))
		if data == nil {
			return core.ErrNoContent
		}
		result = string(data)
		return nil
	})
	return result, err
}

// GetSSRContent retrieves the actual content for an SSR artifact
func (manager *Manager) GetSSRContent(ssrType string, artifact *core.SSRArtifact) ([]byte, error) {
	if len(artifact.InlineContent) > 0 {
		return artifact.InlineContent, nil
	}
	category := filepath.Join("ssr", ssrType)
	return manager.store.Get(category, artifact.OutputHash, artifact.IsCompressed)
}

// GetHTMLContent retrieves HTML content for a post
func (manager *Manager) GetHTMLContent(post *core.PostMeta) ([]byte, error) {
	if len(post.InlineHTML) > 0 {
		return post.InlineHTML, nil
	}
	if post.HTMLHash == "" {
		return nil, core.ErrNoContent
	}
	return manager.store.Get("html", post.HTMLHash, true)
}

// GetPostsByTaxonomy returns post IDs for a given taxonomy and term.
func (manager *Manager) GetPostsByTaxonomy(taxonomy, term string) ([]string, error) {
	prefix := []byte(taxonomy + "/" + term + "/")
	var ids []string

	err := manager.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketTaxonomies))
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for k, _ := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cursor.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})

	return ids, err
}

// GetAllPostsMetadata retrieves minimal metadata for all posts
func (manager *Manager) GetAllPostsMetadata() ([]PostListMeta, error) {
	var result []PostListMeta

	err := manager.db.View(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil
		}

		return postsBucket.ForEach(func(key, value []byte) error {
			var meta core.PostMeta
			if err := core.Decode(value, &meta); err == nil {
				result = append(result, PostListMeta{
					Title:      meta.Title,
					Link:       meta.Link,
					Weight:     meta.Weight,
					Date:       meta.Date,
					Taxonomies: meta.Taxonomies,
				})
			}
			return nil
		})
	})

	return result, err
}

type rawItem struct {
	id   string
	data []byte
}

func (manager *Manager) fetchRawItems(postIDs []string) ([]rawItem, error) {
	rawItems := make([]rawItem, 0, len(postIDs))

	err := manager.db.View(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return core.ErrNoContent
		}

		for _, id := range postIDs {
			data := postsBucket.Get([]byte(id))
			if data != nil {
				// Copy data out of transaction (BoltDB mmap is valid only during tx)
				copied := make([]byte, len(data))
				copy(copied, data)
				rawItems = append(rawItems, rawItem{id: id, data: copied})
			}
		}
		return nil
	})

	return rawItems, err
}
