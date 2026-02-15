package cache

import (
	"encoding/binary"
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
				PostID: []byte(p.PostID),
				Data:   postData,
				Path:   []byte(utils.NormalizePath(p.Path)),
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

	for _, ep := range encoded {
		ops.posts = append(ops.posts, batchOp{key: ep.PostID, value: ep.Data})
		ops.paths = append(ops.paths, batchOp{key: ep.Path, value: ep.PostID})

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

	err := m.db.Update(func(tx *bolt.Tx) error {
		if err := writeOps(tx.Bucket([]byte(BucketPosts)), ops.posts); err != nil {
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

	// Invalidate memory cache for committed posts
	if err == nil {
		for _, ep := range encoded {
			m.memCacheDelete("id:" + string(ep.PostID))
			m.memCacheDelete("path:" + string(ep.Path))
		}
	}

	return err
}

// StoreHTML stores HTML content and returns its hash
func (m *Manager) StoreHTML(content []byte) (string, error) {
	hash, _, err := m.store.Put("html", content)
	return hash, err
}

// StoreHTMLForPost stores HTML for a specific post, inlining if small
func (m *Manager) StoreHTMLForPost(post *PostMeta, content []byte) error {
	if len(content) < utils.InlineHTMLThreshold {
		post.InlineHTML = content
		post.HTMLHash = ""
		return nil
	}
	hash, _, err := m.store.Put("html", content)
	if err != nil {
		return err
	}
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

	err := m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))
		searchBucket := tx.Bucket([]byte(BucketSearch))
		depsBucket := tx.Bucket([]byte(BucketPostDeps))
		tagsBucket := tx.Bucket([]byte(BucketTags))

		postIDBytes := []byte(postID)

		data := postsBucket.Get(postIDBytes)
		if data != nil {
			var post PostMeta
			if err := Decode(data, &post); err == nil {
				postPath = utils.NormalizePath(post.Path)
				_ = pathsBucket.Delete([]byte(postPath))

				for _, tag := range post.Tags {
					tagKey := []byte(tag + "/" + postID)
					_ = tagsBucket.Delete(tagKey)
				}
			}
		}

		_ = postsBucket.Delete(postIDBytes)
		_ = searchBucket.Delete(postIDBytes)
		_ = depsBucket.Delete(postIDBytes)

		return nil
	})

	// Invalidate memory cache
	if err == nil {
		m.memCacheDelete("id:" + postID)
		if postPath != "" {
			m.memCacheDelete("path:" + postPath)
		}
	}

	return err
}
