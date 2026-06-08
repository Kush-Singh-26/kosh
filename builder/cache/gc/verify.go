package gc

import (
	"fmt"
	"path/filepath"

	"go.etcd.io/bbolt"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

const quickVerifySampleSize = 10

// QuickVerify performs a fast integrity check by sampling entries
func QuickVerify(db *bbolt.DB, cacheStore *store.Store) ([]string, error) {
	var verificationErrors []string
	sampleCount := 0

	err := db.View(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		if postsBucket == nil {
			return nil // Empty cache is valid
		}

		cursor := postsBucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if sampleCount >= quickVerifySampleSize {
				break
			}
			sampleCount++

			var post core.ContentMeta
			if err := core.Decode(value, &post); err != nil {
				verificationErrors = append(verificationErrors, fmt.Sprintf("corrupt post data: %s", string(key)))
				continue
			}

			// Check HTML blob exists if referenced
			if post.HTMLHash != "" && !cacheStore.Exists("html", post.HTMLHash) {
				verificationErrors = append(verificationErrors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.ContentID))
			}
		}

		return nil
	})

	return verificationErrors, err
}

// Verify checks cache integrity
func Verify(db *bbolt.DB, cacheStore *store.Store) ([]string, error) {
	var verificationErrors []string

	err := db.View(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		pathsBucket := tx.Bucket([]byte(core.BucketPaths))

		return postsBucket.ForEach(func(key, value []byte) error {
			var post core.ContentMeta
			if err := core.Decode(value, &post); err != nil {
				verificationErrors = append(verificationErrors, fmt.Sprintf("corrupt post data: %s", string(key)))
				return nil
			}

			normalizedPath := fspkg.NormalizePath(post.Path)
			mappedID := pathsBucket.Get([]byte(normalizedPath))
			if mappedID == nil {
				verificationErrors = append(verificationErrors, fmt.Sprintf("missing path mapping: %s -> %s", normalizedPath, post.ContentID))
			} else if string(mappedID) != post.ContentID {
				verificationErrors = append(verificationErrors, fmt.Sprintf("path mapping mismatch: %s -> %s (expected %s)", normalizedPath, string(mappedID), post.ContentID))
			}

			if post.HTMLHash != "" && !cacheStore.Exists("html", post.HTMLHash) {
				verificationErrors = append(verificationErrors, fmt.Sprintf("missing HTML blob: %s for post %s", post.HTMLHash, post.ContentID))
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	err = db.View(func(tx *bbolt.Tx) error {
		ssrBucket := tx.Bucket([]byte(core.BucketSSR))
		return ssrBucket.ForEach(func(key, value []byte) error {
			var artifact core.SSRArtifact
			if err := core.Decode(value, &artifact); err != nil {
				verificationErrors = append(verificationErrors, fmt.Sprintf("corrupt SSR artifact: %s", string(key)))
				return nil
			}

			category := filepath.Join("ssr", artifact.Type)
			if !cacheStore.Exists(category, artifact.OutputHash) {
				verificationErrors = append(verificationErrors, fmt.Sprintf("missing SSR blob: %s for %s", artifact.OutputHash, string(key)))
			}

			return nil
		})
	})

	return verificationErrors, err
}
