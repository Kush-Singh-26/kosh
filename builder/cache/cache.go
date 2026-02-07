package cache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Manager provides the main cache interface
type Manager struct {
	db       *bolt.DB
	store    *Store
	basePath string
	cacheID  string
	mu       sync.RWMutex
	dirty    map[string]bool // track dirty PostIDs for batch commit
}

// Open opens or creates a cache at the given path
func Open(basePath string) (*Manager, error) {
	// Ensure directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Open BoltDB
	dbPath := filepath.Join(basePath, "meta.db")
	db, err := bolt.Open(dbPath, 0644, &bolt.Options{
		Timeout:      1 * time.Second,
		NoGrowSync:   false,
		FreelistType: bolt.FreelistMapType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

	// Create store
	storePath := filepath.Join(basePath, "store")
	store, err := NewStore(storePath)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	m := &Manager{
		db:       db,
		store:    store,
		basePath: basePath,
		dirty:    make(map[string]bool),
	}

	// Initialize schema
	if err := m.initSchema(); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return m, nil
}

// Close closes the cache
func (m *Manager) Close() error {
	if m.store != nil {
		_ = m.store.Close()
	}
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// initSchema creates all buckets if they don't exist
func (m *Manager) initSchema() error {
	return m.db.Update(func(tx *bolt.Tx) error {
		for _, name := range AllBuckets() {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", name, err)
			}
		}

		// Set schema version if not exists
		meta := tx.Bucket([]byte(BucketMeta))
		if meta.Get([]byte(KeySchemaVersion)) == nil {
			v := make([]byte, 4)
			binary.BigEndian.PutUint32(v, SchemaVersion)
			if err := meta.Put([]byte(KeySchemaVersion), v); err != nil {
				return err
			}
		}

		return nil
	})
}

// VerifyCacheID checks if the cache ID matches and returns whether a rebuild is needed
func (m *Manager) VerifyCacheID(expectedID string) (needsRebuild bool, err error) {
	var storedID []byte
	err = m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		storedID = meta.Get([]byte(KeyCacheID))
		return nil
	})
	if err != nil {
		return false, err
	}

	if storedID == nil || string(storedID) != expectedID {
		m.cacheID = expectedID
		return true, nil
	}

	m.cacheID = expectedID
	return false, nil
}

// SetCacheID updates the cache ID
func (m *Manager) SetCacheID(id string) error {
	m.cacheID = id
	return m.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		return meta.Put([]byte(KeyCacheID), []byte(id))
	})
}

// GetPostByPath looks up a post by its file path
func (m *Manager) GetPostByPath(path string) (*PostMeta, error) {
	normalizedPath := normalizePath(path)

	var postMeta *PostMeta
	err := m.db.View(func(tx *bolt.Tx) error {
		paths := tx.Bucket([]byte(BucketPaths))
		postID := paths.Get([]byte(normalizedPath))
		if postID == nil {
			return nil
		}

		posts := tx.Bucket([]byte(BucketPosts))
		data := posts.Get(postID)
		if data == nil {
			return nil
		}

		postMeta = &PostMeta{}
		return Decode(data, postMeta)
	})

	return postMeta, err
}

// GetPostByID retrieves a post by its PostID
func (m *Manager) GetPostByID(postID string) (*PostMeta, error) {
	var postMeta *PostMeta
	err := m.db.View(func(tx *bolt.Tx) error {
		posts := tx.Bucket([]byte(BucketPosts))
		data := posts.Get([]byte(postID))
		if data == nil {
			return nil
		}

		postMeta = &PostMeta{}
		return Decode(data, postMeta)
	})

	return postMeta, err
}

// GetSearchRecord retrieves the search record for a post
func (m *Manager) GetSearchRecord(postID string) (*SearchRecord, error) {
	var record *SearchRecord
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSearch))
		data := bucket.Get([]byte(postID))
		if data == nil {
			return nil
		}

		record = &SearchRecord{}
		return Decode(data, record)
	})

	return record, err
}

// GetDependencies retrieves dependencies for a post
func (m *Manager) GetDependencies(postID string) (*Dependencies, error) {
	var deps *Dependencies
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketPostDeps))
		data := bucket.Get([]byte(postID))
		if data == nil {
			return nil
		}

		deps = &Dependencies{}
		return Decode(data, deps)
	})

	return deps, err
}

// GetSSRArtifact retrieves an SSR artifact
func (m *Manager) GetSSRArtifact(ssrType, inputHash string) (*SSRArtifact, error) {
	key := ssrType + ":" + inputHash

	var artifact *SSRArtifact
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSSR))
		data := bucket.Get([]byte(key))
		if data == nil {
			return nil
		}

		artifact = &SSRArtifact{}
		return Decode(data, artifact)
	})

	return artifact, err
}

// GetSSRContent retrieves the actual content for an SSR artifact
func (m *Manager) GetSSRContent(ssrType string, artifact *SSRArtifact) ([]byte, error) {
	category := filepath.Join("ssr", ssrType)
	return m.store.Get(category, artifact.OutputHash, artifact.Compressed)
}

// GetHTMLContent retrieves HTML content for a post
func (m *Manager) GetHTMLContent(post *PostMeta) ([]byte, error) {
	if post.HTMLHash == "" {
		return nil, nil
	}
	return m.store.Get("html", post.HTMLHash, true) // Try compressed first
}

// MarkDirty marks a PostID as dirty for batch commit
func (m *Manager) MarkDirty(postID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirty[postID] = true
}

// IsDirty checks if a PostID is marked dirty
func (m *Manager) IsDirty(postID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dirty[postID]
}

// BatchCommit commits all pending changes in a single transaction
func (m *Manager) BatchCommit(posts []*PostMeta, searchRecords map[string]*SearchRecord, deps map[string]*Dependencies) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))
		searchBucket := tx.Bucket([]byte(BucketSearch))
		depsBucket := tx.Bucket([]byte(BucketPostDeps))
		tagsBucket := tx.Bucket([]byte(BucketTags))

		for _, post := range posts {
			postID := []byte(post.PostID)
			normalizedPath := []byte(normalizePath(post.Path))

			// Update posts bucket
			data, err := Encode(post)
			if err != nil {
				return fmt.Errorf("failed to encode post: %w", err)
			}
			if err := postsBucket.Put(postID, data); err != nil {
				return err
			}

			// Update paths mapping
			if err := pathsBucket.Put(normalizedPath, postID); err != nil {
				return err
			}

			// Update search record
			if sr, ok := searchRecords[post.PostID]; ok {
				data, err := Encode(sr)
				if err != nil {
					return err
				}
				if err := searchBucket.Put(postID, data); err != nil {
					return err
				}
			}

			// Update dependencies
			if d, ok := deps[post.PostID]; ok {
				data, err := Encode(d)
				if err != nil {
					return err
				}
				if err := depsBucket.Put(postID, data); err != nil {
					return err
				}

				// Update tag indexes
				for _, tag := range d.Tags {
					tagKey := []byte(tag + "/" + post.PostID)
					if err := tagsBucket.Put(tagKey, nil); err != nil {
						return err
					}
				}

				// Update template indexes
				depsTemplatesBucket := tx.Bucket([]byte(BucketDepsTemplates))
				for _, tmpl := range d.Templates {
					tmplKey := []byte(tmpl + "/" + post.PostID)
					if err := depsTemplatesBucket.Put(tmplKey, nil); err != nil {
						return err
					}
				}

				// Update include indexes
				depsIncludesBucket := tx.Bucket([]byte(BucketDepsIncludes))
				for _, inc := range d.Includes {
					incKey := []byte(inc + "/" + post.PostID)
					if err := depsIncludesBucket.Put(incKey, nil); err != nil {
						return err
					}
				}
			}
		}

		// Update build stats
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
}

// StoreHTML stores HTML content and returns its hash
func (m *Manager) StoreHTML(content []byte) (string, error) {
	hash, _, err := m.store.Put("html", content)
	return hash, err
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

	// Store metadata
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
	return m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))
		searchBucket := tx.Bucket([]byte(BucketSearch))
		depsBucket := tx.Bucket([]byte(BucketPostDeps))
		tagsBucket := tx.Bucket([]byte(BucketTags))

		postIDBytes := []byte(postID)

		// Get post to find path
		data := postsBucket.Get(postIDBytes)
		if data != nil {
			var post PostMeta
			if err := Decode(data, &post); err == nil {
				// Remove path mapping
				_ = pathsBucket.Delete([]byte(normalizePath(post.Path)))

				// Remove tag indexes
				for _, tag := range post.Tags {
					tagKey := []byte(tag + "/" + postID)
					_ = tagsBucket.Delete(tagKey)
				}
			}
		}

		// Delete from all buckets
		_ = postsBucket.Delete(postIDBytes)
		_ = searchBucket.Delete(postIDBytes)
		_ = depsBucket.Delete(postIDBytes)

		return nil
	})
}

// ListAllPosts returns all PostIDs
func (m *Manager) ListAllPosts() ([]string, error) {
	var ids []string
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketPosts))
		return bucket.ForEach(func(k, _ []byte) error {
			ids = append(ids, string(k))
			return nil
		})
	})
	return ids, err
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

// Stats returns current cache statistics
func (m *Manager) Stats() (*CacheStats, error) {
	stats := &CacheStats{
		SchemaVersion: SchemaVersion,
	}

	// Count posts
	err := m.db.View(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		stats.TotalPosts = postsBucket.Stats().KeyN

		ssrBucket := tx.Bucket([]byte(BucketSSR))
		stats.TotalSSR = ssrBucket.Stats().KeyN

		statsBucket := tx.Bucket([]byte(BucketStats))
		if data := statsBucket.Get([]byte(KeyBuildCount)); data != nil {
			stats.BuildCount = int(binary.BigEndian.Uint32(data))
		}
		if data := statsBucket.Get([]byte(KeyLastGC)); data != nil {
			stats.LastGC = int64(binary.BigEndian.Uint64(data))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Get store sizes
	htmlSize, _ := m.store.Size("html")
	d2Size, _ := m.store.Size(filepath.Join("ssr", "d2"))
	katexSize, _ := m.store.Size(filepath.Join("ssr", "katex"))
	stats.StoreBytes = htmlSize + d2Size + katexSize

	return stats, nil
}

// Store returns the underlying content store
func (m *Manager) Store() *Store {
	return m.store
}

// DB returns the underlying BoltDB instance (for advanced operations)
func (m *Manager) DB() *bolt.DB {
	return m.db
}

// GetSocialCardHash retrieves the hash for a social card
func (m *Manager) GetSocialCardHash(path string) (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSocialCard))
		data := bucket.Get([]byte(path))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetSocialCardHash stores the hash for a social card
func (m *Manager) SetSocialCardHash(path, hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSocialCard))
		return bucket.Put([]byte(path), []byte(hash))
	})
}

// GetGraphHash retrieves the graph data hash
func (m *Manager) GetGraphHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		data := meta.Get([]byte(KeyGraphHash))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetGraphHash stores the graph data hash
func (m *Manager) SetGraphHash(hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		return meta.Put([]byte(KeyGraphHash), []byte(hash))
	})
}

// GetAllSocialCardHashes returns all social card hashes
func (m *Manager) GetAllSocialCardHashes() (map[string]string, error) {
	hashes := make(map[string]string)
	err := m.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketSocialCard))
		return bucket.ForEach(func(k, v []byte) error {
			hashes[string(k)] = string(v)
			return nil
		})
	})
	return hashes, err
}

// normalizePath normalizes a file path for consistent cache keys
func normalizePath(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "content/")
	return strings.ToLower(path)
}

// GetWasmHash retrieves the stored WASM source hash
func (m *Manager) GetWasmHash() (string, error) {
	var hash string
	err := m.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		data := meta.Get([]byte(KeyWasmHash))
		if data != nil {
			hash = string(data)
		}
		return nil
	})
	return hash, err
}

// SetWasmHash stores the WASM source hash
func (m *Manager) SetWasmHash(hash string) error {
	return m.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(BucketMeta))
		return meta.Put([]byte(KeyWasmHash), []byte(hash))
	})
}
