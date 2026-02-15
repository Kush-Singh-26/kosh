package cache

import (
	"fmt"
	"path/filepath"

	bolt "go.etcd.io/bbolt"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

const quickVerifySampleSize = 10

// QuickVerify performs a fast integrity check by sampling entries
// Returns errors found and any error during verification
func (m *Manager) QuickVerify() ([]string, error) {
	var errors []string
	sampleCount := 0

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		if postsBucket == nil {
			return nil // Empty cache is valid
		}

		cursor := postsBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			if sampleCount >= quickVerifySampleSize {
				break
			}
			sampleCount++

			var post PostMeta
			if err := Decode(v, &post); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt post data: %s", string(k)))
				continue
			}

			// Check HTML blob exists if referenced
			if post.HTMLHash != "" && !m.store.Exists("html", post.HTMLHash) {
				errors = append(errors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.PostID))
			}
		}

		return nil
	})

	return errors, err
}

// Verify checks cache integrity
func (m *Manager) Verify() ([]string, error) {
	var errors []string

	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))

		return postsBucket.ForEach(func(k, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt post data: %s", string(k)))
				return nil
			}

			normalizedPath := utils.NormalizePath(post.Path)
			mappedID := pathsBucket.Get([]byte(normalizedPath))
			if mappedID == nil {
				errors = append(errors, fmt.Sprintf("missing path mapping: %s -> %s", normalizedPath, post.PostID))
			} else if string(mappedID) != post.PostID {
				errors = append(errors, fmt.Sprintf("path mapping mismatch: %s -> %s (expected %s)", normalizedPath, string(mappedID), post.PostID))
			}

			if post.HTMLHash != "" && !m.store.Exists("html", post.HTMLHash) {
				errors = append(errors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.PostID))
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	err = m.db.View(func(tx *bolt.Tx) error {
		ssrBucket := tx.Bucket([]byte(BucketSSR))
		return ssrBucket.ForEach(func(k, v []byte) error {
			var artifact SSRArtifact
			if err := Decode(v, &artifact); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt SSR artifact: %s", string(k)))
				return nil
			}

			category := filepath.Join("ssr", artifact.Type)
			if !m.store.Exists(category, artifact.OutputHash) {
				errors = append(errors, fmt.Sprintf("missing SSR blob: %s for %s", artifact.OutputHash, string(k)))
			}

			return nil
		})
	})

	return errors, err
}
