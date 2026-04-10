package cache

// MarkDirty marks a PostID as dirty for batch commit
func (manager *Manager) MarkDirty(postID string) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.dirty[postID] = true
}

// IsDirty checks if a PostID is marked dirty
func (manager *Manager) IsDirty(postID string) bool {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	return manager.dirty[postID]
}

// ClearDirty clears all dirty flags
func (manager *Manager) ClearDirty() {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.dirty = make(map[string]bool)
}
