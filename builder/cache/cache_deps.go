package cache

import (
	"bytes"

	bolt "go.etcd.io/bbolt"
)

// GetPostsByTemplate returns all PostIDs that depend on a given template
func (m *Manager) GetPostsByTemplate(templatePath string) ([]string, error) {
	prefix := []byte(templatePath + "/")
	var ids []string

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketDepsTemplates))
		c := bucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})

	return ids, err
}

// GetPostsByInclude returns all PostIDs that depend on a given include (shortcode)
func (m *Manager) GetPostsByInclude(includePath string) ([]string, error) {
	prefix := []byte(includePath + "/")
	var ids []string

	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketDepsIncludes))
		c := bucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			postID := string(k[len(prefix):])
			ids = append(ids, postID)
		}
		return nil
	})

	return ids, err
}
