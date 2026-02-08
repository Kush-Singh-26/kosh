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

// encodedPostPool is a sync.Pool for reusing EncodedPost slices
// This reduces GC pressure during batch commits
var encodedPostPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate with a reasonable capacity for typical batch sizes
		return make([]EncodedPost, 0, 64)
	},
}

// Manager provides the main cache interface
type Manager struct {
	db       *bolt.DB
	store    *Store
	basePath string
	cacheID  string
	mu       sync.RWMutex
	dirty    map[string]bool // track dirty PostIDs for batch commit
	// Performance tracking
	stats cacheStatsInternal
}

// cacheStatsInternal holds runtime performance metrics
type cacheStatsInternal struct {
	lastReadTime  time.Duration
	lastWriteTime time.Duration
	readCount     int64
	writeCount    int64
}

// Open opens or creates a cache at the given path
// isDev: when true, uses faster but less durable settings (safe for dev mode)
func Open(basePath string, isDev bool) (*Manager, error) {
	// Ensure directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Configure BoltDB options based on dev/prod mode
	opts := &bolt.Options{
		Timeout:         1 * time.Second,
		FreelistType:    bolt.FreelistArrayType, // Better for sequential access
		PageSize:        16384,                  // Larger pages for HTML content
		InitialMmapSize: 10 * 1024 * 1024,       // 10MB initial mmap
	}

	if isDev {
		// Dev mode: faster, slightly less durable (acceptable for dev)
		opts.NoGrowSync = true // Skip fsync on database growth
		// Note: NoSync and NoFreelistSync can be enabled for even faster dev builds
		// but are disabled by default for safety
	} else {
		// Production mode: fully durable
		opts.NoGrowSync = false
	}

	// Open BoltDB
	dbPath := filepath.Join(basePath, "meta.db")
	db, err := bolt.Open(dbPath, 0644, opts)
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

// GetPostsByIDs retrieves multiple posts by their PostIDs in a single transaction
// Optimized to avoid N+1 query problem
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
				continue // Skip corrupted entries
			}
			result[id] = &postMeta
		}
		return nil
	})

	return result, err
}

// GetSearchRecords retrieves multiple search records by PostIDs in a single transaction
// Optimized for batch operations during template-only rebuilds
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
				continue // Skip corrupted entries
			}
			result[id] = &record
		}
		return nil
	})

	return result, err
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
// Optimized: checks for inline HTML first (avoids 2nd I/O for small posts)
func (m *Manager) GetHTMLContent(post *PostMeta) ([]byte, error) {
	// Fast path: inline HTML for small posts
	if len(post.InlineHTML) > 0 {
		return post.InlineHTML, nil
	}
	// Fallback: content-addressed storage for large posts
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

// EncodedPost holds pre-encoded data for batch commit
type EncodedPost struct {
	PostID     []byte
	Data       []byte
	Path       []byte
	SearchData []byte
	DepsData   []byte
	Tags       []string
	Templates  []string
	Includes   []string
}

// BatchCommit commits all pending changes in a single transaction
// Optimized: Pre-encodes all data outside transaction for faster writes
func (m *Manager) BatchCommit(posts []*PostMeta, searchRecords map[string]*SearchRecord, deps map[string]*Dependencies) error {
	start := time.Now()

	// Pre-encode all data OUTSIDE the transaction (can be parallelized)
	// Use sync.Pool to reuse EncodedPost slices and reduce GC pressure
	encoded := encodedPostPool.Get().([]EncodedPost)[:0]
	defer func() {
		// Return slice to pool for reuse (clear references to help GC)
		for i := range encoded {
			encoded[i] = EncodedPost{}
		}
		encodedPostPool.Put(encoded)
	}()

	for _, post := range posts {
		postData, err := Encode(post)
		if err != nil {
			return fmt.Errorf("failed to encode post: %w", err)
		}

		ep := EncodedPost{
			PostID: []byte(post.PostID),
			Data:   postData,
			Path:   []byte(normalizePath(post.Path)),
		}

		// Pre-encode search record if exists
		if sr, ok := searchRecords[post.PostID]; ok {
			srData, err := Encode(sr)
			if err != nil {
				return err
			}
			ep.SearchData = srData
		}

		// Pre-encode dependencies and extract index data
		if d, ok := deps[post.PostID]; ok {
			depsData, err := Encode(d)
			if err != nil {
				return err
			}
			ep.DepsData = depsData
			ep.Tags = d.Tags
			ep.Templates = d.Templates
			ep.Includes = d.Includes
		}

		encoded = append(encoded, ep)
	}

	err := m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		pathsBucket := tx.Bucket([]byte(BucketPaths))
		searchBucket := tx.Bucket([]byte(BucketSearch))
		depsBucket := tx.Bucket([]byte(BucketPostDeps))
		tagsBucket := tx.Bucket([]byte(BucketTags))
		depsTemplatesBucket := tx.Bucket([]byte(BucketDepsTemplates))
		depsIncludesBucket := tx.Bucket([]byte(BucketDepsIncludes))

		for _, ep := range encoded {
			// Main post data
			if err := postsBucket.Put(ep.PostID, ep.Data); err != nil {
				return err
			}

			// Path mapping
			if err := pathsBucket.Put(ep.Path, ep.PostID); err != nil {
				return err
			}

			// Search record
			if ep.SearchData != nil {
				if err := searchBucket.Put(ep.PostID, ep.SearchData); err != nil {
					return err
				}
			}

			// Dependencies and indexes
			if ep.DepsData != nil {
				if err := depsBucket.Put(ep.PostID, ep.DepsData); err != nil {
					return err
				}

				// Tag indexes
				for _, tag := range ep.Tags {
					tagKey := []byte(tag + "/" + string(ep.PostID))
					if err := tagsBucket.Put(tagKey, nil); err != nil {
						return err
					}
				}

				// Template indexes
				for _, tmpl := range ep.Templates {
					tmplKey := []byte(tmpl + "/" + string(ep.PostID))
					if err := depsTemplatesBucket.Put(tmplKey, nil); err != nil {
						return err
					}
				}

				// Include indexes
				for _, inc := range ep.Includes {
					incKey := []byte(inc + "/" + string(ep.PostID))
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

	// Track write timing metrics
	if err == nil {
		writeTime := time.Since(start)
		m.mu.Lock()
		m.stats.lastWriteTime = writeTime
		m.stats.writeCount++
		m.mu.Unlock()
	}

	return err
}

// StoreHTML stores HTML content and returns its hash
// For small content (< 32KB), it stores inline to avoid 2nd I/O
func (m *Manager) StoreHTML(content []byte) (string, error) {
	hash, _, err := m.store.Put("html", content)
	return hash, err
}

// StoreHTMLForPost stores HTML for a specific post, inlining if small
// Updates post.InlineHTML or post.HTMLHash accordingly
func (m *Manager) StoreHTMLForPost(post *PostMeta, content []byte) error {
	if len(content) < InlineHTMLThreshold {
		// Inline small HTML directly in PostMeta
		post.InlineHTML = content
		post.HTMLHash = "" // Clear hash since we're inlining
		return nil
	}
	// Large content: use content-addressed storage
	hash, _, err := m.store.Put("html", content)
	if err != nil {
		return err
	}
	post.HTMLHash = hash
	post.InlineHTML = nil // Clear inline
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
	start := time.Now()
	stats := &CacheStats{
		SchemaVersion: SchemaVersion,
	}

	// Count posts and inline/hashed distribution
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

		// Count inline vs hashed posts
		return postsBucket.ForEach(func(k, v []byte) error {
			var post PostMeta
			if err := Decode(v, &post); err == nil {
				if len(post.InlineHTML) > 0 {
					stats.InlinePosts++
				} else if post.HTMLHash != "" {
					stats.HashedPosts++
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	// Get store sizes
	htmlSize, _ := m.store.Size("html")
	d2Size, _ := m.store.Size(filepath.Join("ssr", "d2"))
	katexSize, _ := m.store.Size(filepath.Join("ssr", "katex"))
	stats.StoreBytes = htmlSize + d2Size + katexSize

	// Update read timing metrics
	readTime := time.Since(start)
	m.mu.Lock()
	m.stats.lastReadTime = readTime
	m.stats.readCount++
	stats.LastReadTime = m.stats.lastReadTime
	stats.LastWriteTime = m.stats.lastWriteTime
	stats.ReadCount = m.stats.readCount
	stats.WriteCount = m.stats.writeCount
	m.mu.Unlock()

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
// Optimized to reduce allocations using strings.Builder
func normalizePath(path string) string {
	// Fast path: no content/ prefix and no backslashes
	if !strings.Contains(path, "\\") && !strings.HasPrefix(path, "content/") {
		return strings.ToLower(path)
	}

	var b strings.Builder
	b.Grow(len(path))

	// Normalize separators and remove prefix in one pass
	skipContent := strings.HasPrefix(path, "content/") || strings.HasPrefix(path, "content\\")
	start := 0
	if skipContent {
		start = 8 // len("content/")
	}

	for i := start; i < len(path); i++ {
		c := path[i]
		if c == '\\' {
			b.WriteByte('/')
		} else if c >= 'A' && c <= 'Z' {
			b.WriteByte(c + 32) // ToLower without function call
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
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
