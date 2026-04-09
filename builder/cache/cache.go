package cache

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	"github.com/Kush-Singh-26/kosh/builder/cache/migrate"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"
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
	mu       sync.RWMutex // protects dirty and cacheID
	dirty    map[string]bool

	// In-memory LRU cache for hot PostMeta data
	memCache    *lru.Cache[string, *memoryCacheEntry]
	memCacheTTL time.Duration

	// Reference counting for content-addressed storage
	refCount *gc.RefCountManager
}

const defaultMemCacheTTL = 5 * time.Minute

const (
	defaultOpenTimeout     = 10 * time.Second
	cacheDirMode           = 0755
	dbFileMode             = 0644
	defaultInitialMmapSize = 10 * 1024 * 1024
	maxInitialMmapSize     = 100 * 1024 * 1024
	mmapSizeMultiplier     = 2
	boltPageSize           = 16384
	memCacheMaxEntries     = 1024
	uint32Size             = 4
)

// Open opens or creates a cache at the given path
func Open(basePath string, isDev bool) (*Manager, error) {
	return OpenWithTimeout(basePath, isDev, defaultOpenTimeout)
}

func computeInitialMmapSize(dbPath string) int {
	initialSize := defaultInitialMmapSize
	if info, err := os.Stat(dbPath); err == nil {
		calculatedSize := int(info.Size()) * mmapSizeMultiplier
		if calculatedSize > maxInitialMmapSize {
			initialSize = maxInitialMmapSize
		} else if calculatedSize > defaultInitialMmapSize {
			initialSize = calculatedSize
		}
	}
	return initialSize
}

func openBoltDB(dbPath string, timeout time.Duration, initialSize int) (*bbolt.DB, error) {
	opts := &bbolt.Options{
		Timeout:         timeout,
		FreelistType:    bbolt.FreelistArrayType,
		PageSize:        boltPageSize,
		InitialMmapSize: initialSize,
		NoGrowSync:      true,
		NoSync:          true,
	}
	return bbolt.Open(dbPath, dbFileMode, opts)
}

func openStoreWithCleanup(db *bbolt.DB, basePath string) (*store.Store, error) {
	storePath := filepath.Join(basePath, "store")
	st, err := store.New(storePath)
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Failed to close DB during cleanup", "error", closeErr)
		}
		return nil, err
	}
	return st, nil
}

func newMemCacheWithCleanup(st *store.Store, db *bbolt.DB) (*lru.Cache[string, *memoryCacheEntry], error) {
	cache, err := lru.New[string, *memoryCacheEntry](memCacheMaxEntries)
	if err != nil {
		if closeErr := st.Close(); closeErr != nil {
			slog.Error("Failed to close store during cleanup", "error", closeErr)
		}
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("Failed to close DB during cleanup", "error", closeErr)
		}
		return nil, err
	}
	return cache, nil
}

func loadSchemaVersion(db *bbolt.DB) (uint32, error) {
	var currentVersion uint32
	err := db.View(func(tx *bbolt.Tx) error {
		meta := tx.Bucket([]byte(core.BucketMeta))
		if meta != nil {
			v := meta.Get([]byte(core.KeySchemaVersion))
			if v != nil {
				currentVersion = binary.BigEndian.Uint32(v)
			}
		}
		return nil
	})
	return currentVersion, err
}

func ensureSchemaVersion(db *bbolt.DB, currentVersion uint32) error {
	if currentVersion == 0 || currentVersion == uint32(core.SchemaVersion) {
		return nil
	}
	newVer, err := migrate.RunMigrations(db, currentVersion, nil)
	if err != nil || newVer != uint32(core.SchemaVersion) {
		return fmt.Errorf("incompatible schema version: got %d, want %d (migration failed: %w)", currentVersion, core.SchemaVersion, err)
	}
	return nil
}

// OpenWithTimeout opens or creates a cache with a custom timeout
func OpenWithTimeout(basePath string, isDev bool, timeout time.Duration) (*Manager, error) {
	if err := os.MkdirAll(basePath, cacheDirMode); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(basePath, "meta.db")
	initialSize := computeInitialMmapSize(dbPath)

	// Kosh cache is derivative and reproducible from source.
	// Skipping fsync (NoSync) and mmap grow sync (NoGrowSync) significantly
	// improves build performance, especially on Windows, with minimal durability risk
	// for a build tool.
	db, err := openBoltDB(dbPath, timeout, initialSize)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

	store, err := openStoreWithCleanup(db, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	lruCache, err := newMemCacheWithCleanup(store, db)
	if err != nil {
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
		if cleanupErr := m.cleanupOnError(); cleanupErr != nil {
			slog.Error("Failed to cleanup after schema init failure", "error", cleanupErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Verify schema and run migrations if needed
	currentVersion, err := loadSchemaVersion(m.db)
	if err != nil {
		if cleanupErr := m.cleanupOnError(); cleanupErr != nil {
			slog.Error("Failed to cleanup after schema version read failure", "error", cleanupErr)
		}
		return nil, fmt.Errorf("failed to read schema version: %w", err)
	}

	if err := ensureSchemaVersion(m.db, currentVersion); err != nil {
		if cleanupErr := m.cleanupOnError(); cleanupErr != nil {
			slog.Error("Failed to cleanup after migration failure", "error", cleanupErr)
		}
		return nil, err
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
		return fmt.Errorf("cleanup errors: %w", errors.Join(errs...))
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
		return fmt.Errorf("cache close errors: %w", errors.Join(errs...))
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
			v := make([]byte, uint32Size)
			binary.BigEndian.PutUint32(v, uint32(core.SchemaVersion))
			if err := meta.Put([]byte(core.KeySchemaVersion), v); err != nil {
				return err
			}
		}

		return nil
	})
}

// VerifyCacheID checks if the cache ID matches
func (m *Manager) VerifyCacheID(expectedID string) (bool, error) {
	var storedID []byte
	err := m.db.View(func(tx *bbolt.Tx) error {
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
			if err := tx.DeleteBucket([]byte(name)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
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
	categories := []string{
		"html",
		"search",
		"ssr/d2",
		"ssr/math",
		"ssr/math-inline",
		"ssr/math-block",
		"ssr/katex",
	}
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

// Store exposes the underlying content-addressed store.
func (m *Manager) Store() *store.Store {
	return m.store
}

// DB exposes the underlying BoltDB handle.
func (m *Manager) DB() *bbolt.DB {
	return m.db
}

// RunGC performs garbage collection
func (m *Manager) RunGC(cfg gc.GCConfig) (*gc.GCResult, error) {
	return gc.RunGC(m.db, m.store, m.refCount, cfg)
}

// IncrementBuildCount increments the counter used to trigger GC
func (m *Manager) IncrementBuildCount() (uint32, error) {
	var count uint32
	err := m.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(core.BucketStats))

		// Increment global build count
		var buildCount uint32
		if data := bucket.Get([]byte(core.KeyBuildCount)); data != nil {
			buildCount = binary.BigEndian.Uint32(data)
		}
		buildCount++
		buildCountData := make([]byte, uint32Size)
		binary.BigEndian.PutUint32(buildCountData, buildCount)
		if err := bucket.Put([]byte(core.KeyBuildCount), buildCountData); err != nil {
			return err
		}

		// Increment builds since last GC
		data := bucket.Get([]byte("builds_since_gc"))
		if data != nil {
			count = binary.BigEndian.Uint32(data)
		}
		count++
		newData := make([]byte, uint32Size)
		binary.BigEndian.PutUint32(newData, count)
		return bucket.Put([]byte("builds_since_gc"), newData)
	})
	return count, err
}

// QuickVerify performs a fast integrity check by sampling entries
func (m *Manager) QuickVerify() ([]string, error) {
	return gc.QuickVerify(m.db, m.store)
}

// Verify checks cache integrity
func (m *Manager) Verify() ([]string, error) {
	return gc.Verify(m.db, m.store)
}
