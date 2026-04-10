package cache

import (
	"os"

	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
)

// Clear removes all cache data
func (manager *Manager) Clear() error {
	_ = manager.db.Close()

	_ = os.RemoveAll(manager.basePath)

	newManager, err := Open(manager.basePath, false)
	if err != nil {
		return err
	}

	manager.db = newManager.db
	manager.store = newManager.store
	manager.dirty = make(map[string]bool)
	manager.refCount = gc.NewRefCountManager(manager.db)
	manager.memCache.Purge()

	return nil
}

// Rebuild triggers a full cache rebuild by clearing the cache
func (manager *Manager) Rebuild() error {
	return manager.Clear()
}
