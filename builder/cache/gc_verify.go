package cache

import (
	"fmt"
	"path/filepath"

	bolt "go.etcd.io/bbolt"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

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
