package cache

import (
	"bytes"
	"path/filepath"

	bolt "go.etcd.io/bbolt"

	"my-ssg/builder/utils"
)

// getCachedItem retrieves a generic item from a bucket
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

// GetPostByPath looks up a post by its file path
func (m *Manager) GetPostByPath(path string) (*PostMeta, error) {
	normalizedPath := utils.NormalizePath(path)

	var postID []byte
	err := m.db.View(func(tx *bolt.Tx) error {
		paths := tx.Bucket([]byte(BucketPaths))
		postID = paths.Get([]byte(normalizedPath))
		return nil
	})
	if err != nil || postID == nil {
		return nil, err
	}

	return getCachedItem[PostMeta](m.db, BucketPosts, postID)
}

// GetPostByID retrieves a post by its PostID
func (m *Manager) GetPostByID(postID string) (*PostMeta, error) {
	return getCachedItem[PostMeta](m.db, BucketPosts, []byte(postID))
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

			var postMeta PostMeta
			if err := Decode(data, &postMeta); err != nil {
				continue
			}
			result[id] = &postMeta
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

// GetDependencies retrieves dependencies for a post
func (m *Manager) GetDependencies(postID string) (*Dependencies, error) {
	return getCachedItem[Dependencies](m.db, BucketPostDeps, []byte(postID))
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
