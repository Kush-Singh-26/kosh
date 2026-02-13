package cache

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// encodedPostPool is a sync.Pool for reusing EncodedPost slices
var encodedPostPool = sync.Pool{
	New: func() interface{} {
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
	dirty    map[string]bool
	stats    cacheStatsInternal
}

// cacheStatsInternal holds runtime performance metrics
type cacheStatsInternal struct {
	lastReadTime  time.Duration
	lastWriteTime time.Duration
	readCount     int64
	writeCount    int64
}

// Open opens or creates a cache at the given path
func Open(basePath string, isDev bool) (*Manager, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	opts := &bolt.Options{
		Timeout:         10 * time.Second,
		FreelistType:    bolt.FreelistArrayType,
		PageSize:        16384,
		InitialMmapSize: 10 * 1024 * 1024,
	}

	if isDev {
		opts.NoGrowSync = true
	} else {
		opts.NoGrowSync = false
	}

	dbPath := filepath.Join(basePath, "meta.db")
	db, err := bolt.Open(dbPath, 0644, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

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

// VerifyCacheID checks if the cache ID matches
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

// Store returns the underlying content store
func (m *Manager) Store() *Store {
	return m.store
}

// DB returns the underlying BoltDB instance
func (m *Manager) DB() *bolt.DB {
	return m.db
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
}

// writeOps performs sequential writes to a bucket
func writeOps(bucket *bolt.Bucket, ops []batchOp) error {
	for _, op := range ops {
		if err := bucket.Put(op.key, op.value); err != nil {
			return err
		}
	}
	return nil
}
