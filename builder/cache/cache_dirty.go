package cache

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
