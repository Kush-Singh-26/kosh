package cache

// MarkDirty marks a contentID as dirty for batch commit
func (manager *Manager) MarkDirty(contentID string) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.dirty[contentID] = true
}

// IsDirty checks if a contentID is marked dirty
func (manager *Manager) IsDirty(contentID string) bool {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	return manager.dirty[contentID]
}

// ClearDirty clears all dirty flags
func (manager *Manager) ClearDirty() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.dirty = make(map[string]bool)
}
