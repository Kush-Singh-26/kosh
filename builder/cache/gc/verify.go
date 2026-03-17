package gc

import (
	"fmt"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	bolt "go.etcd.io/bbolt"
)

const quickVerifySampleSize = 10

// QuickVerify performs a fast integrity check by sampling entries
func QuickVerify(db *bolt.DB, s *store.Store) ([]string, error) {
	var errors []string
	sampleCount := 0

	err := db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil // Empty cache is valid
		}

		cursor := postsBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			if sampleCount >= quickVerifySampleSize {
				break
			}
			sampleCount++

			var post core.PostMeta
			if err := core.Decode(v, &post); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt post data: %s", string(k)))
				continue
			}

			// Check HTML blob exists if referenced
			if post.HTMLHash != "" && !s.Exists("html", post.HTMLHash) {
				errors = append(errors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.PostID))
			}
		}

		return nil
	})

	return errors, err
}

// Verify checks cache integrity
func Verify(db *bolt.DB, s *store.Store) ([]string, error) {
	var errors []string

	err := db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		pathsBucket := tx.Bucket([]byte(core.BucketPaths))

		return postsBucket.ForEach(func(k, v []byte) error {
			var post core.PostMeta
			if err := core.Decode(v, &post); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt post data: %s", string(k)))
				return nil
			}

			normalizedPath := fspkg.NormalizePath(post.Path)
			mappedID := pathsBucket.Get([]byte(normalizedPath))
			if mappedID == nil {
				errors = append(errors, fmt.Sprintf("missing path mapping: %s -> %s", normalizedPath, post.PostID))
			} else if string(mappedID) != post.PostID {
				errors = append(errors, fmt.Sprintf("path mapping mismatch: %s -> %s (expected %s)", normalizedPath, string(mappedID), post.PostID))
			}

			if post.HTMLHash != "" && !s.Exists("html", post.HTMLHash) {
				errors = append(errors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.PostID))
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	err = db.View(func(tx *bolt.Tx) error {
		ssrBucket := tx.Bucket([]byte(core.BucketSSR))
		return ssrBucket.ForEach(func(k, v []byte) error {
			var artifact core.SSRArtifact
			if err := core.Decode(v, &artifact); err != nil {
				errors = append(errors, fmt.Sprintf("corrupt SSR artifact: %s", string(k)))
				return nil
			}

			category := filepath.Join("ssr", artifact.Type)
			if !s.Exists(category, artifact.OutputHash) {
				errors = append(errors, fmt.Sprintf("missing SSR blob: %s for %s", artifact.OutputHash, string(k)))
			}

			return nil
		})
	})

	return errors, err
}
