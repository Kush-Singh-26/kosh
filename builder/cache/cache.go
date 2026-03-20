package cache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	"github.com/Kush-Singh-26/kosh/builder/cache/migrate"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.etcd.io/bbolt"
)

// memoryCacheEntry holds a cached core.PostMeta with expiration
type memoryCacheEntry struct {
	meta      *core.PostMeta
	expiresAt time.Time
}

// Manager provides the main cache interface
type Manager struct {
	db       *bbolt.DB
	store    *store.Store
	basePath string
	cacheID  string
	mu       sync.RWMutex
	dirty    map[string]bool

	// In-memory LRU cache for hot PostMeta data
	memCache    *lru.Cache[string, *memoryCacheEntry]
	memCacheTTL time.Duration

	// Reference counting for content-addressed storage
	refCount *gc.RefCountManager
}

const defaultMemCacheTTL = 5 * time.Minute

// Open opens or creates a cache at the given path
func Open(basePath string, isDev bool) (*Manager, error) {
	return OpenWithTimeout(basePath, isDev, 10*time.Second)
}

// OpenWithTimeout opens or creates a cache with a custom timeout
func OpenWithTimeout(basePath string, isDev bool, timeout time.Duration) (*Manager, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Calculate initial mmap size based on existing database
	initialSize := 10 * 1024 * 1024 // Default 10MB
	dbPath := filepath.Join(basePath, "meta.db")
	if info, err := os.Stat(dbPath); err == nil {
		// Use 2x current size, minimum 10MB, maximum 100MB
		calculatedSize := int(info.Size()) * 2
		if calculatedSize > 100*1024*1024 {
			initialSize = 100 * 1024 * 1024
		} else if calculatedSize > 10*1024*1024 {
			initialSize = calculatedSize
		}
	}

	opts := &bbolt.Options{
		Timeout:         timeout,
		FreelistType:    bbolt.FreelistArrayType,
		PageSize:        16384,
		InitialMmapSize: initialSize,
	}

	if isDev {
		opts.NoGrowSync = true
	} else {
		opts.NoGrowSync = false
	}

	db, err := bbolt.Open(dbPath, 0644, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

	storePath := filepath.Join(basePath, "store")
	store, err := store.New(storePath)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	lruCache, err := lru.New[string, *memoryCacheEntry](1024) // 1024 items max
	if err != nil {
		_ = store.Close()
		_ = db.Close()
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	m := &Manager{
		db:          db,
		store:       store,
		basePath:    basePath,
		dirty:       make(map[string]bool),
		memCache:    lruCache,
		memCacheTTL: defaultMemCacheTTL,
		refCount:    gc.NewRefCountManager(db),
	}

	if err := m.initSchema(); err != nil {
		_ = m.cleanupOnError()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Verify schema and run migrations if needed
	var currentVersion uint32
	err = m.db.View(func(tx *bbolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		if meta != nil {
			v := meta.Get([]byte(core.KeySchemaVersion))
			if v != nil {
				currentVersion = binary.BigEndian.Uint32(v)
			}
		}
		return nil
	})
	if err != nil {
		_ = m.cleanupOnError()
		return nil, fmt.Errorf("failed to read schema version: %w", err)
	}

	if currentVersion > 0 && currentVersion != uint32(core.SchemaVersion) {
		newVer, err := migrate.RunMigrations(m.db, currentVersion, nil)
		if err != nil || newVer != uint32(core.SchemaVersion) {
			_ = m.cleanupOnError()
			return nil, fmt.Errorf("incompatible schema version: got %d, want %d (migration failed: %w)", currentVersion, core.SchemaVersion, err)
		}
	}

	return m, nil
}

// cleanupOnError closes all resources during initialization failure
func (m *Manager) cleanupOnError() error {
	var errs []error
	if m.store != nil {
		if err := m.store.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}

// Close closes the cache
func (m *Manager) Close() error {
	var errs []error
	if m.store != nil {
		if err := m.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close store: %w", err))
		}
	}
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close db: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cache close errors: %v", errs)
	}
	return nil
}

// initSchema creates all buckets if they don't exist
func (m *Manager) initSchema() error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		for _, name := range core.AllBuckets() {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", name, err)
			}
		}

		meta := tx.Bucket([]byte(core.BucketMeta))
		if meta.Get([]byte(core.KeySchemaVersion)) == nil {
			v := make([]byte, 4)
			binary.BigEndian.PutUint32(v, uint32(core.SchemaVersion))
			if err := meta.Put([]byte(core.KeySchemaVersion), v); err != nil {
				return err
			}
		}

		return nil
	})
}

// VerifyCacheID checks if the cache ID matches
func (m *Manager) VerifyCacheID(expectedID string) (needsRebuild bool, err error) {
	var storedID []byte
	err = m.db.View(func(tx *bbolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		storedID = meta.Get([]byte(core.KeyCacheID))
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
	return m.db.Update(func(tx *bbolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		return meta.Put([]byte(core.KeyCacheID), []byte(id))
	})
}

// ClearAll removes all cached data (used when corruption detected)
func (m *Manager) ClearAll() error {
	err := m.db.Update(func(tx *bbolt.Tx) error {
		for _, name := range core.AllBuckets() {
			if name == core.BucketMeta {
				continue // Keep metadata
			}
			// Drop and recreate the bucket — O(1) vs O(N) key-by-key delete
			if err := tx.DeleteBucket([]byte(name)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) { //nolint:staticcheck // bbolt.ErrBucketNotFound is deprecated in 1.4+
				return err
			}
			if _, err := tx.CreateBucket([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})

	// Clear memory cache
	m.memCache.Purge()

	// Clear filesystem store
	if err := m.clearFilesystemStore(); err != nil {
		// Log but don't fail - the DB clear is the critical part
		slog.Warn("Failed to clear filesystem store", "error", err)
	}

	return err
}

// clearFilesystemStore removes all content from the filesystem store
func (m *Manager) clearFilesystemStore() error {
	// List all categories and delete their contents
	categories := []string{"html", "ssr-d2", "ssr-math", "search"}
	for _, category := range categories {
		hashes, err := m.store.ListHashes(category)
		if err != nil {
			continue // Skip categories that don't exist
		}
		for _, hash := range hashes {
			_ = m.store.Delete(category, hash)
		}
	}
	return nil
}

func (m *Manager) Store() *store.Store {
	return m.store
}

func (m *Manager) DB() *bbolt.DB {
	return m.db
}

// RunGC performs garbage collection
func (m *Manager) RunGC(cfg gc.GCConfig) (*gc.GCResult, error) {
	return gc.RunGC(m.db, m.store, m.refCount, cfg)
}

// QuickVerify performs a fast integrity check by sampling entries
func (m *Manager) QuickVerify() ([]string, error) {
	return gc.QuickVerify(m.db, m.store)
}

// Verify checks cache integrity
func (m *Manager) Verify() ([]string, error) {
	return gc.Verify(m.db, m.store)
}

// EncodedPost holds pre-encoded data for batch commit
type EncodedPost struct {
	PostID     []byte
	Data       []byte
	Path       []byte
	SearchData []byte
	DepsData   []byte
	Version    string
	Tags       []string
	Templates  []string
	Includes   []string
}

// batchOp represents a single key-value operation for bucket writes
type batchOp struct {
	key   []byte
	value []byte
}

// bucketOps groups all operations by bucket for sequential writes
type bucketOps struct {
	posts     []batchOp
	paths     []batchOp
	search    []batchOp
	deps      []batchOp
	tags      []batchOp
	templates []batchOp
	includes  []batchOp
	versions  []batchOp
}

// writeOps performs sequential writes to a bucket
func writeOps(bucket *bbolt.Bucket, ops []batchOp) error {
	if bucket == nil {
		return nil
	}
	for _, op := range ops {
		if err := bucket.Put(op.key, op.value); err != nil {
			return err
		}
	}
	return nil
}

// sortOps sorts a slice of batch operations by key for sequential write performance
func sortOps(ops []batchOp) {
	sort.Slice(ops, func(i, j int) bool {
		return bytes.Compare(ops[i].key, ops[j].key) < 0
	})
}
