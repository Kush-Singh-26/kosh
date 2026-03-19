package cache

import (
	"encoding/binary"
	"os"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"

	bolt "go.etcd.io/bbolt"
)

// Clear removes all cache data
func (m *Manager) Clear() error {
	_ = m.db.Close()

	_ = os.RemoveAll(m.basePath)

	newManager, err := Open(m.basePath, false)
	if err != nil {
		return err
	}

	m.db = newManager.db
	m.store = newManager.store
	m.dirty = make(map[string]bool)
	m.refCount = gc.NewRefCountManager(m.db)
	m.memCache.Purge()

	return nil
}

// Rebuild triggers a full cache rebuild by clearing the cache
func (m *Manager) Rebuild() error {
	return m.Clear()
}

// IncrementBuildCount increments the build counter
func (m *Manager) IncrementBuildCount() error {
	return m.db.Update(func(tx *bolt.Tx) error {
		statsBucket := tx.Bucket([]byte(core.BucketStats))

		buildCount := uint32(1)
		if data := statsBucket.Get([]byte(core.KeyBuildCount)); data != nil {
			buildCount = binary.BigEndian.Uint32(data) + 1
		}
		countData := make([]byte, 4)
		binary.BigEndian.PutUint32(countData, buildCount)
		if err := statsBucket.Put([]byte(core.KeyBuildCount), countData); err != nil {
			return err
		}

		buildsSinceGC := uint32(1)
		if data := statsBucket.Get([]byte("builds_since_gc")); data != nil {
			buildsSinceGC = binary.BigEndian.Uint32(data) + 1
		}
		sinceGCData := make([]byte, 4)
		binary.BigEndian.PutUint32(sinceGCData, buildsSinceGC)
		return statsBucket.Put([]byte("builds_since_gc"), sinceGCData)
	})
}
