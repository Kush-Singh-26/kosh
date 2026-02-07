package cache

import (
	"sync"
)

// DiagramCacheAdapter provides a map[string]string interface backed by BoltDB
// This allows the existing markdown parser to work with the new cache system
type DiagramCacheAdapter struct {
	manager *Manager
	local   map[string]string // In-memory buffer for current build
	mu      sync.RWMutex
}

// NewDiagramCacheAdapter creates a new adapter
func NewDiagramCacheAdapter(manager *Manager) *DiagramCacheAdapter {
	return &DiagramCacheAdapter{
		manager: manager,
		local:   make(map[string]string),
	}
}

// Get retrieves a cached diagram
func (a *DiagramCacheAdapter) Get(key string) (string, bool) {
	a.mu.RLock()
	if val, ok := a.local[key]; ok {
		a.mu.RUnlock()
		return val, true
	}
	a.mu.RUnlock()

	// Try to get from BoltDB if manager is available
	if a.manager != nil {
		// Parse key to extract type and hash (format: "{hash}_{theme}")
		// For now, try as-is
		artifact, err := a.manager.GetSSRArtifact("d2", key)
		if err == nil && artifact != nil {
			content, err := a.manager.GetSSRContent("d2", artifact)
			if err == nil {
				result := string(content)
				a.mu.Lock()
				a.local[key] = result
				a.mu.Unlock()
				return result, true
			}
		}
	}

	return "", false
}

// Set stores a diagram in the cache
func (a *DiagramCacheAdapter) Set(key string, value string) {
	a.mu.Lock()
	a.local[key] = value
	a.mu.Unlock()

	// Also store in BoltDB if manager is available
	if a.manager != nil {
		go func() {
			_, _ = a.manager.StoreSSR("d2", key, []byte(value))
		}()
	}
}

// Flush writes all local entries to BoltDB
func (a *DiagramCacheAdapter) Flush() error {
	if a.manager == nil {
		return nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	for key, value := range a.local {
		_, err := a.manager.StoreSSR("d2", key, []byte(value))
		if err != nil {
			return err
		}
	}

	return nil
}

// AsMap returns the local cache as a map (for compatibility)
func (a *DiagramCacheAdapter) AsMap() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]string, len(a.local))
	for k, v := range a.local {
		result[k] = v
	}
	return result
}

// Clear clears the local cache
func (a *DiagramCacheAdapter) Clear() {
	a.mu.Lock()
	a.local = make(map[string]string)
	a.mu.Unlock()
}
