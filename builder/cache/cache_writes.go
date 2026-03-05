package cache

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// BatchCommit commits all pending changes in a single transaction
func (m *Manager) BatchCommit(posts []*PostMeta, searchRecords map[string]*SearchRecord, deps map[string]*Dependencies) error {
	// Pre-allocate slice for parallel encoding results
	encoded := make([]EncodedPost, len(posts))

	// Parallelize encoding for better performance
	var encodeWg sync.WaitGroup
	var encodeMu sync.Mutex
	var encodeErr error

	for i, post := range posts {
		encodeWg.Add(1)
		go func(idx int, p *PostMeta) {
			defer encodeWg.Done()

			postData, err := Encode(p)
			if err != nil {
				encodeMu.Lock()
				if encodeErr == nil {
					encodeErr = err
				}
				encodeMu.Unlock()
				return
			}

			ep := EncodedPost{
				PostID:  []byte(p.PostID),
				Data:    postData,
				Path:    []byte(utils.NormalizePath(p.Path)),
				Version: p.Version,
			}

			if sr, ok := searchRecords[p.PostID]; ok {
				srData, err := Encode(sr)
				if err != nil {
					encodeMu.Lock()
					if encodeErr == nil {
						encodeErr = err
					}
					encodeMu.Unlock()
					return
				}
				ep.SearchData = srData
			}

			if d, ok := deps[p.PostID]; ok {
				depsData, err := Encode(d)
				if err != nil {
					encodeMu.Lock()
					if encodeErr == nil {
						encodeErr = err
					}
					encodeMu.Unlock()
					return
				}
				ep.DepsData = depsData
				ep.Tags = d.Tags
				ep.Templates = d.Templates
				ep.Includes = d.Includes
			}

			encoded[idx] = ep
		}(i, post)
	}
	encodeWg.Wait()

	if encodeErr != nil {
		return encodeErr
	}

	var ops bucketOps
	totalTags := 0
	totalTemplates := 0
	totalIncludes := 0
	for _, ep := range encoded {
		totalTags += len(ep.Tags)
		totalTemplates += len(ep.Templates)
		totalIncludes += len(ep.Includes)
	}

	ops.posts = make([]batchOp, 0, len(encoded))
	ops.paths = make([]batchOp, 0, len(encoded))
	ops.search = make([]batchOp, 0, len(encoded))
	ops.deps = make([]batchOp, 0, len(encoded))
	ops.tags = make([]batchOp, 0, totalTags)
	ops.templates = make([]batchOp, 0, totalTemplates)
	ops.includes = make([]batchOp, 0, totalIncludes)
	ops.versions = make([]batchOp, 0, len(encoded))

	for _, ep := range encoded {
		ops.posts = append(ops.posts, batchOp{key: ep.PostID, value: ep.Data})
		ops.paths = append(ops.paths, batchOp{key: ep.Path, value: ep.PostID})

		if ep.Version != "" {
			verKey := []byte(ep.Version + "/" + string(ep.PostID))
			ops.versions = append(ops.versions, batchOp{key: verKey, value: nil})
		}

		if ep.SearchData != nil {
			ops.search = append(ops.search, batchOp{key: ep.PostID, value: ep.SearchData})
		}

		if ep.DepsData != nil {
			ops.deps = append(ops.deps, batchOp{key: ep.PostID, value: ep.DepsData})

			for _, tag := range ep.Tags {
				tagKey := []byte(tag + "/" + string(ep.PostID))
				ops.tags = append(ops.tags, batchOp{key: tagKey, value: nil})
			}

			for _, tmpl := range ep.Templates {
				tmplKey := []byte(tmpl + "/" + string(ep.PostID))
				ops.templates = append(ops.templates, batchOp{key: tmplKey, value: nil})
			}

			for _, inc := range ep.Includes {
				incKey := []byte(inc + "/" + string(ep.PostID))
				ops.includes = append(ops.includes, batchOp{key: incKey, value: nil})
			}
		}
	}

	// Invalidate memory cache BEFORE the transaction
	for _, ep := range encoded {
		m.memCacheDelete("id:" + string(ep.PostID))
		m.memCacheDelete("path:" + string(ep.Path))
	}

	err := m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))

		// Phase 1: Collect old HTML hashes for refcount delta (inside the tx)
		oldHashes := make(map[string]string) // postID -> oldHTMLHash
		for _, ep := range encoded {
			if existing := postsBucket.Get(ep.PostID); existing != nil {
				var oldPost PostMeta
				if err := Decode(existing, &oldPost); err == nil && oldPost.HTMLHash != "" {
					oldHashes[string(ep.PostID)] = oldPost.HTMLHash
				}
			}
		}

		// Phase 2: Write all bucket operations
		if err := writeOps(postsBucket, ops.posts); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketPaths)), ops.paths); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketSearch)), ops.search); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketPostDeps)), ops.deps); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketTags)), ops.tags); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketDepsTemplates)), ops.templates); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketDepsIncludes)), ops.includes); err != nil {
			return err
		}
		if err := writeOps(tx.Bucket([]byte(BucketVersions)), ops.versions); err != nil {
			return err
		}

		// Phase 3: Adjust refcounts atomically inside the same transaction
		for _, ep := range encoded {
			var newPost PostMeta
			if err := Decode(ep.Data, &newPost); err != nil {
				continue
			}
			oldHash := oldHashes[string(ep.PostID)]
			newHash := newPost.HTMLHash

			if oldHash != "" && oldHash != newHash {
				if err := m.refCount.DecrementTx(tx, oldHash, nil); err != nil {
					return fmt.Errorf("failed to decrement refcount: %w", err)
				}
			}
			if newHash != "" && newHash != oldHash {
				if err := m.refCount.IncrementTx(tx, newHash); err != nil {
					return fmt.Errorf("failed to increment refcount: %w", err)
				}
			}
		}

		stats := tx.Bucket([]byte(BucketStats))
		buildCount := uint32(1)
		if data := stats.Get([]byte(KeyBuildCount)); data != nil {
			buildCount = binary.BigEndian.Uint32(data) + 1
		}
		countData := make([]byte, 4)
		binary.BigEndian.PutUint32(countData, buildCount)
		if err := stats.Put([]byte(KeyBuildCount), countData); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// Log failed batch commits with post IDs for manual reconciliation
		postIDs := make([]string, len(encoded))
		for i, ep := range encoded {
			postIDs[i] = string(ep.PostID)
		}
		slog.Error("BatchCommit failed", "count", len(postIDs), "ids", postIDs, "error", err)
	}

	return err
}

// StoreHTML stores HTML content and returns its hash
func (m *Manager) StoreHTML(content []byte) (string, error) {
	hash, _, err := m.store.Put("html", content)
	return hash, err
}

// StoreHTMLForPost stores HTML for a specific post, inlining if small.
// Note: Refcount adjustments are handled atomically inside BatchCommit,
// not here. This method only sets the HTMLHash/InlineHTML fields on the post struct.
func (m *Manager) StoreHTMLForPost(post *PostMeta, content []byte) error {
	if len(content) < utils.InlineHTMLThreshold {
		// Small content is inlined directly, no hash needed
		post.InlineHTML = content
		post.HTMLHash = ""
		return nil
	}
	hash, _, err := m.store.Put("html", content)
	if err != nil {
		return err
	}

	// Just set the hash — refcount is reconciled atomically in BatchCommit
	post.HTMLHash = hash
	post.InlineHTML = nil
	return nil
}

// StoreSSR stores an SSR artifact and its content
func (m *Manager) StoreSSR(ssrType, inputHash string, content []byte) (*SSRArtifact, error) {
	category := filepath.Join("ssr", ssrType)
	outputHash, ct, err := m.store.Put(category, content)
	if err != nil {
		return nil, err
	}

	artifact := &SSRArtifact{
		Type:       ssrType,
		InputHash:  inputHash,
		OutputHash: outputHash,
		Size:       int64(len(content)),
		CreatedAt:  time.Now().Unix(),
		Compressed: ct != CompressionNone,
	}

	key := ssrType + ":" + inputHash
	data, err := Encode(artifact)
	if err != nil {
		return nil, err
	}

	err = m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSSR))
		return bucket.Put([]byte(key), data)
	})

	return artifact, err
}

// DeletePost removes a post and its associated data
func (m *Manager) DeletePost(postID string) error {
	var postPath string
	var htmlHash string
	var deleteErrors []error

	err := m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))
		searchBucket := tx.Bucket([]byte(BucketSearch))
		depsBucket := tx.Bucket([]byte(BucketPostDeps))
		tagsBucket := tx.Bucket([]byte(BucketTags))
		versionsBucket := tx.Bucket([]byte(BucketVersions))

		postIDBytes := []byte(postID)

		data := postsBucket.Get(postIDBytes)
		if data != nil {
			var post PostMeta
			if decodeErr := Decode(data, &post); decodeErr == nil {
				postPath = utils.NormalizePath(post.Path)
				htmlHash = post.HTMLHash
				if err := pathsBucket.Delete([]byte(postPath)); err != nil {
					deleteErrors = append(deleteErrors, fmt.Errorf("delete path: %w", err))
				}

				for _, tag := range post.Tags {
					tagKey := []byte(tag + "/" + postID)
					if err := tagsBucket.Delete(tagKey); err != nil {
						deleteErrors = append(deleteErrors, fmt.Errorf("delete tag %s: %w", tag, err))
					}
				}
				if post.Version != "" && versionsBucket != nil {
					verKey := []byte(post.Version + "/" + postID)
					if err := versionsBucket.Delete(verKey); err != nil {
						deleteErrors = append(deleteErrors, fmt.Errorf("delete version: %w", err))
					}
				}
			}
		}

		if err := postsBucket.Delete(postIDBytes); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete post: %w", err))
		}
		if err := searchBucket.Delete(postIDBytes); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete search: %w", err))
		}
		if err := depsBucket.Delete(postIDBytes); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete deps: %w", err))
		}

		// Decrement refcount inside transaction
		if htmlHash != "" {
			if err := m.refCount.DecrementTx(tx, htmlHash, nil); err != nil {
				deleteErrors = append(deleteErrors, fmt.Errorf("decrement refcount: %w", err))
			}
		}

		return nil
	})

	// Log any delete errors (best effort cleanup)
	for _, delErr := range deleteErrors {
		slog.Warn("Cache delete error", "postID", postID, "error", delErr)
	}

	// Invalidate memory cache
	if err == nil {
		m.memCacheDelete("id:" + postID)
		if postPath != "" {
			m.memCacheDelete("path:" + postPath)
		}
	}

	return err
}
