package cache

import (
	"os"

	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
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
