package cache

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"

	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

const (
	ssrKeySplitParts          = 2
	windowsSSRIOConcurrency   = 64
	ssrParallelismMultiplier  = 2
	ssrInlineContentLimitSize = 16 * 1024
)

func encodePosts(posts []*core.PostMeta, searchRecords map[string]*core.SearchRecord, dependencies map[string]*core.Dependencies) ([]EncodedPost, error) {
	encoded := make([]EncodedPost, len(posts))

	var encodeMutex sync.Mutex
	var encodeError error
	var errorGroup errgroup.Group
	errorGroup.SetLimit(runtime.NumCPU())

	for i, post := range posts {
		index, currentPost := i, post
		errorGroup.Go(func() error {
			encodedPost, err := encodeSinglePost(currentPost, searchRecords[currentPost.PostID], dependencies[currentPost.PostID])
			if err != nil {
				encodeMutex.Lock()
				if encodeError == nil {
					encodeError = err
				} else {
					slog.Warn("Additional encode error suppressed", "error", err)
				}
				encodeMutex.Unlock()
				return err
			}
			encoded[index] = encodedPost
			return nil
		})
	}
	_ = errorGroup.Wait()

	if encodeError != nil {
		return nil, encodeError
	}
	return encoded, nil
}

func buildBucketOps(encoded []EncodedPost) bucketOps {
	var ops bucketOps
	totalTaxEntries := 0
	totalTemplates := 0
	totalIncludes := 0
	for _, encodedPost := range encoded {
		for _, terms := range encodedPost.Taxonomies {
			totalTaxEntries += len(terms)
		}
		totalTemplates += len(encodedPost.Templates)
		totalIncludes += len(encodedPost.Includes)
	}

	ops.posts = make([]batchOp, 0, len(encoded))
	ops.paths = make([]batchOp, 0, len(encoded))
	ops.search = make([]batchOp, 0, len(encoded))
	ops.deps = make([]batchOp, 0, len(encoded))
	ops.taxonomies = make([]batchOp, 0, totalTaxEntries)
	ops.templates = make([]batchOp, 0, totalTemplates)
	ops.includes = make([]batchOp, 0, totalIncludes)

	for _, encodedPost := range encoded {
		ops.posts = append(ops.posts, batchOp{key: encodedPost.PostID, value: encodedPost.Data})
		ops.paths = append(ops.paths, batchOp{key: encodedPost.Path, value: encodedPost.PostID})

		if encodedPost.SearchData != nil {
			ops.search = append(ops.search, batchOp{key: encodedPost.PostID, value: encodedPost.SearchData})
		}

		if encodedPost.DepsData != nil {
			ops.deps = append(ops.deps, batchOp{key: encodedPost.PostID, value: encodedPost.DepsData})

			for tax, terms := range encodedPost.Taxonomies {
				for _, term := range terms {
					taxKey := []byte(tax + "/" + term + "/" + string(encodedPost.PostID))
					ops.taxonomies = append(ops.taxonomies, batchOp{key: taxKey, value: nil})
				}
			}

			for _, tmpl := range encodedPost.Templates {
				tmplKey := []byte(tmpl + "/" + string(encodedPost.PostID))
				ops.templates = append(ops.templates, batchOp{key: tmplKey, value: nil})
			}

			for _, inc := range encodedPost.Includes {
				incKey := []byte(inc + "/" + string(encodedPost.PostID))
				ops.includes = append(ops.includes, batchOp{key: incKey, value: nil})
			}
		}
	}
	return ops
}

func invalidateMemCache(manager *Manager, encoded []EncodedPost) {
	for _, encodedPost := range encoded {
		manager.memCacheDelete("id:" + string(encodedPost.PostID))
		manager.memCacheDelete("path:" + string(encodedPost.Path))
	}
}

func collectOldHashes(postsBucket *bbolt.Bucket, encoded []EncodedPost) map[string]string {
	oldHashes := make(map[string]string)
	for _, encodedPost := range encoded {
		if existing := postsBucket.Get(encodedPost.PostID); existing != nil {
			var oldPost core.PostMeta
			if err := core.Decode(existing, &oldPost); err == nil && oldPost.HTMLHash != "" {
				oldHashes[string(encodedPost.PostID)] = oldPost.HTMLHash
			}
		}
	}
	return oldHashes
}

func writeAllOps(tx *bbolt.Tx, ops bucketOps) error {
	sortOps(ops.posts)
	sortOps(ops.paths)
	sortOps(ops.search)
	sortOps(ops.deps)
	sortOps(ops.taxonomies)
	sortOps(ops.templates)
	sortOps(ops.includes)

	if err := writeOps(tx.Bucket([]byte(core.BucketPosts)), ops.posts); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketPaths)), ops.paths); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketSearch)), ops.search); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketPostDeps)), ops.deps); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketTaxonomies)), ops.taxonomies); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketDepsTemplates)), ops.templates); err != nil {
		return err
	}
	if err := writeOps(tx.Bucket([]byte(core.BucketDepsIncludes)), ops.includes); err != nil {
		return err
	}
	return nil
}

func updateRefCounts(tx *bbolt.Tx, refCount *gc.RefCountManager, encoded []EncodedPost, oldHashes map[string]string) error {
	for _, encodedPost := range encoded {
		var newPost core.PostMeta
		if err := core.Decode(encodedPost.Data, &newPost); err != nil {
			continue
		}
		oldHash := oldHashes[string(encodedPost.PostID)]
		newHash := newPost.HTMLHash

		if oldHash != "" && oldHash != newHash {
			if err := refCount.DecrementTx(tx, oldHash, nil); err != nil {
				return fmt.Errorf("failed to decrement refcount: %w", err)
			}
		}
		if newHash != "" && newHash != oldHash {
			if err := refCount.IncrementTx(tx, newHash); err != nil {
				return fmt.Errorf("failed to increment refcount: %w", err)
			}
		}
	}
	return nil
}

func bumpBuildCount(tx *bbolt.Tx) error {
	stats := tx.Bucket([]byte(core.BucketStats))
	buildCount := uint32(1)
	if data := stats.Get([]byte(core.KeyBuildCount)); data != nil {
		buildCount = binary.BigEndian.Uint32(data) + 1
	}
	countData := make([]byte, uint32Size)
	binary.BigEndian.PutUint32(countData, buildCount)
	return stats.Put([]byte(core.KeyBuildCount), countData)
}

func logBatchCommitFailure(err error, encoded []EncodedPost) {
	postIDs := make([]string, len(encoded))
	for i, ep := range encoded {
		postIDs[i] = string(ep.PostID)
	}
	slog.Error("BatchCommit failed", "count", len(postIDs), "ids", postIDs, "error", err)
}

// BatchCommit atomically commits posts, search records, and dependencies in a single BoltDB transaction.
// All data is encoded in parallel with bounded concurrency before the transaction begins.
//
// Error Contract:
//   - Returns error on BoltDB transaction failure or encoding error (partial commit not possible)
//   - Retry behavior: Safe to retry on failure; idempotent within same build session
//   - Thread safety: Concurrent calls are serialized via internal mutex
//   - On error, no data is committed (all-or-nothing semantics)
//
// Usage Note:
// BatchCommit is designed for asynchronous fire-and-forget calls from the build pipeline.
// It is safe to call without checking the error - the caller should log the error for visibility,
// but the build should continue. On failure, the cache will rebuild on the next run.
func (manager *Manager) BatchCommit(posts []*core.PostMeta, searchRecords map[string]*core.SearchRecord, dependencies map[string]*core.Dependencies) error {
	encoded, err := encodePosts(posts, searchRecords, dependencies)
	if err != nil {
		return err
	}

	ops := buildBucketOps(encoded)

	// Invalidate memory cache BEFORE the transaction
	invalidateMemCache(manager, encoded)

	err = manager.db.Update(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))

		// Phase 1: Collect old HTML hashes for refcount delta (inside the tx)
		oldHashes := collectOldHashes(postsBucket, encoded)

		// Phase 2: Write all bucket operations
		// Sort operations by key for sequential write performance in BoltDB
		if err := writeAllOps(tx, ops); err != nil {
			return err
		}

		// Phase 3: Adjust refcounts atomically inside the same transaction
		if err := updateRefCounts(tx, manager.refCount, encoded, oldHashes); err != nil {
			return err
		}

		return bumpBuildCount(tx)
	})

	if err != nil {
		// Log failed batch commits with post IDs for manual reconciliation
		logBatchCommitFailure(err, encoded)
	}

	return err
}

// StoreHTML stores HTML content and returns its hash
func (manager *Manager) StoreHTML(content []byte) (string, error) {
	hash, _, err := manager.store.Put("html", content)
	return hash, err
}

// StoreFragment persists a pre-rendered UI fragment in the cache.
func (manager *Manager) StoreFragment(key string, html string) error {
	return manager.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketFragments))
		if bucket == nil {
			return fmt.Errorf("fragment bucket not found")
		}
		return bucket.Put([]byte(key), []byte(html))
	})
}

// StoreHTMLForPost stores HTML for a specific post, inlining if small.
// Note: Refcount adjustments are handled atomically inside BatchCommit,
// not here. This method only sets the HTMLHash/InlineHTML fields on the post struct.
func (manager *Manager) StoreHTMLForPost(post *core.PostMeta, content []byte) error {
	if len(content) < models.InlineHTMLThreshold {
		// Small content is inlined directly, no hash needed
		post.InlineHTML = content
		post.HTMLHash = ""
		return nil
	}
	hash, _, err := manager.store.Put("html", content)
	if err != nil {
		return err
	}

	// Just set the hash — refcount is reconciled atomically in BatchCommit
	post.HTMLHash = hash
	post.InlineHTML = nil
	return nil
}

// StoreSSR stores an SSR artifact and its content
func (manager *Manager) StoreSSR(ssrType, inputHash string, content []byte) (*core.SSRArtifact, error) {
	category := filepath.Join("ssr", ssrType)
	outputHash, compressionType, err := manager.store.Put(category, content)
	if err != nil {
		return nil, err
	}

	artifact := &core.SSRArtifact{
		Type:       ssrType,
		InputHash:  inputHash,
		OutputHash: outputHash,
		Size:       int64(len(content)),
		CreatedAt:  time.Now().Unix(),
		IsCompressed: compressionType != core.CompressionNone,
	}

	key := ssrType + ":" + inputHash
	data, err := core.Encode(artifact)
	if err != nil {
		return nil, err
	}

	err = manager.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketSSR))
		return bucket.Put([]byte(key), data)
	})

	return artifact, err
}

// BatchStoreSSR stores multiple SSR artifacts and their contents in parallel.
// It uses a single BoltDB transaction for all metadata updates and parallel file writes.
// Entries values are expected to be string, []byte, or JSON-marshalable types
// (for example models.SSRThemePair).
func (manager *Manager) BatchStoreSSR(ctx context.Context, entries map[string]any) error {
	if len(entries) == 0 {
		return nil
	}

	type ssrResult struct {
		key  string
		data []byte
	}

	results := make([]ssrResult, 0, len(entries))
	var resultsMutex sync.Mutex

	errorGroup, groupCtx := errgroup.WithContext(ctx)
	// Higher concurrency on Windows helps overlap slow file I/O for large diagrams
	if runtime.GOOS == "windows" {
		errorGroup.SetLimit(windowsSSRIOConcurrency)
	} else {
		errorGroup.SetLimit(runtime.NumCPU() * ssrParallelismMultiplier)
	}

	for key, value := range entries {
		currentKey, currentValue := key, value
		errorGroup.Go(func() error {
			processedKey, data, err := manager.processSSREntry(groupCtx, currentKey, currentValue)
			if err != nil {
				return err
			}
			resultsMutex.Lock()
			results = append(results, ssrResult{key: processedKey, data: data})
			resultsMutex.Unlock()
			return nil
		})
	}

	if err := errorGroup.Wait(); err != nil {
		return err
	}

	// Single transaction for all metadata
	return manager.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketSSR))
		for _, res := range results {
			if err := bucket.Put([]byte(res.key), res.data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (manager *Manager) processSSREntry(_ context.Context, key string, value any) (string, []byte, error) {
	ssrType := "d2"
	inputHash := key
	if parts := strings.SplitN(key, ":", ssrKeySplitParts); len(parts) == ssrKeySplitParts {
		ssrType = parts[0]
		inputHash = parts[1]
	}

	var content []byte
	var err error
	switch valType := value.(type) {
	case string:
		content = []byte(valType)
	case []byte:
		content = valType
	default:
		content, err = json.Marshal(valType)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal SSR value for %s: %w", key, err)
		}
	}

	artifact := &core.SSRArtifact{
		Type:      ssrType,
		InputHash: inputHash,
		Size:      int64(len(content)),
		CreatedAt: time.Now().Unix(),
	}

	// Inline content under 16KB directly in BoltDB to avoid slow Windows file I/O
	if len(content) < ssrInlineContentLimitSize {
		artifact.InlineContent = content
		artifact.IsCompressed = false
		artifact.OutputHash = core.HashContent(content)
	} else {
		category := filepath.Join("ssr", ssrType)
		outputHash, compressionType, err := manager.store.Put(category, content)
		if err != nil {
			return "", nil, fmt.Errorf("failed to store SSR content for %s: %w", key, err)
		}
		artifact.OutputHash = outputHash
		artifact.IsCompressed = compressionType != core.CompressionNone
	}

	data, err := core.Encode(artifact)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode SSR artifact for %s: %w", key, err)
	}

	return ssrType + ":" + inputHash, data, nil
}

func encodeSinglePost(p *core.PostMeta, sr *core.SearchRecord, d *core.Dependencies) (EncodedPost, error) {
	postData, err := core.Encode(p)
	if err != nil {
		return EncodedPost{}, err
	}

	ep := EncodedPost{
		PostID: []byte(p.PostID),
		Data:   postData,
		Path:   []byte(fspkg.NormalizePath(p.Path)),
	}

	if sr != nil {
		srData, err := core.Encode(sr)
		if err != nil {
			return EncodedPost{}, err
		}
		ep.SearchData = srData
	}

	if d != nil {
		depsData, err := core.Encode(d)
		if err != nil {
			return EncodedPost{}, err
		}
		ep.DepsData = depsData
		ep.Taxonomies = d.Taxonomies
		ep.Templates = d.Templates
		ep.Includes = d.Includes
	}

	return ep, nil
}

// DeletePost removes a post and its associated data
func (manager *Manager) DeletePost(postID string) error {
	postPath, _, deleteErrors, err := manager.deletePostInTx(postID)

	// Log any delete errors (best effort cleanup)
	for _, delErr := range deleteErrors {
		slog.Warn("Cache delete error", "postID", postID, "error", delErr)
	}

	// Invalidate memory cache
	if err == nil {
		manager.memCacheDelete("id:" + postID)
		if postPath != "" {
			manager.memCacheDelete("path:" + postPath)
		}
	}

	return err
}

func (manager *Manager) deletePostInTx(postID string) (string, string, []error, error) {
	var postPath string
	var htmlHash string
	var deleteErrors []error

	err := manager.db.Update(func(tx *bbolt.Tx) error {
		postsBucket := tx.Bucket([]byte(core.BucketPosts))
		pathsBucket := tx.Bucket([]byte(core.BucketPaths))
		searchBucket := tx.Bucket([]byte(core.BucketSearch))
		depsBucket := tx.Bucket([]byte(core.BucketPostDeps))
		taxonomiesBucket := tx.Bucket([]byte(core.BucketTaxonomies))

		postIDBytes := []byte(postID)

		data := postsBucket.Get(postIDBytes)
		if data != nil {
			var post core.PostMeta
			if decodeErr := core.Decode(data, &post); decodeErr == nil {
				postPath = fspkg.NormalizePath(post.Path)
				htmlHash = post.HTMLHash
				if err := pathsBucket.Delete([]byte(postPath)); err != nil {
					deleteErrors = append(deleteErrors, fmt.Errorf("delete path: %w", err))
				}

				for tax, terms := range post.Taxonomies {
					for _, term := range terms {
						taxKey := []byte(tax + "/" + term + "/" + postID)
						if err := taxonomiesBucket.Delete(taxKey); err != nil {
							deleteErrors = append(deleteErrors, fmt.Errorf("delete tax %s/%s: %w", tax, term, err))
						}
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
			if err := manager.refCount.DecrementTx(tx, htmlHash, nil); err != nil {
				deleteErrors = append(deleteErrors, fmt.Errorf("decrement refcount: %w", err))
			}
		}

		return nil
	})

	return postPath, htmlHash, deleteErrors, err
}
