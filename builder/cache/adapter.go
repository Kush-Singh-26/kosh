package cache

import (
	"encoding/json"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// DiagramCacheAdapter provides a map[string]any interface backed by BoltDB
// This allows the existing markdown parser to work with the new cache system.
// It caches values in memory during a build and flushes dirty entries to BoltDB in a single batch.
type DiagramCacheAdapter struct {
	manager *Manager
	// local values are SSR payloads: string (HTML/SVG) or models.SSRThemePair.
	local map[string]any
	// dirty values mirror local and are flushed to BoltDB on Flush().
	dirty   map[string]any
	mu      sync.RWMutex
	closed  atomic.Bool // Prevents new operations after Close() is called
	persist atomic.Bool // Controls whether writes are persisted to disk
}

// NewDiagramCacheAdapter creates a new adapter.
func NewDiagramCacheAdapter(manager *Manager) *DiagramCacheAdapter {
	a := &DiagramCacheAdapter{
		manager: manager,
		local:   make(map[string]any),
		dirty:   make(map[string]any),
	}
	a.persist.Store(true)
	return a
}

// Start is a no-op kept for interface compatibility.
func (a *DiagramCacheAdapter) Start() {}

// Get retrieves a cached diagram.
func (a *DiagramCacheAdapter) Get(key string) (any, bool) {
	a.mu.RLock()
	if val, ok := a.local[key]; ok {
		a.mu.RUnlock()
		return val, true
	}
	a.mu.RUnlock()

	// Try to get from BoltDB if manager is available
	if a.manager != nil {
		category := "d2"
		actualKey := key
		if parts := strings.SplitN(key, ":", 2); len(parts) == 2 {
			category = parts[0]
			actualKey = parts[1]
		}
		artifact, err := a.manager.GetSSRArtifact(category, actualKey)
		if err == nil && artifact != nil {
			content, err := a.manager.GetSSRContent(category, artifact)
			if err == nil {
				var result any
				// Try to unmarshal as SSRThemePair first if it looks like JSON
				if len(content) > 0 && content[0] == '{' {
					var pair models.SSRThemePair
					if err := json.Unmarshal(content, &pair); err == nil {
						result = pair
					} else {
						result = string(content)
					}
				} else {
					result = string(content)
				}

				a.mu.Lock()
				a.local[key] = result
				a.mu.Unlock()
				return result, true
			}
		}
	}

	return nil, false
}

// GetLocal retrieves a cached diagram from in-memory state only.
// It avoids BoltDB lookups and is intended for hot-path render checks.
func (a *DiagramCacheAdapter) GetLocal(key string) (any, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	val, ok := a.local[key]
	return val, ok
}

// Set stores a diagram in the memory cache and marks it dirty for later flushing.
func (a *DiagramCacheAdapter) Set(key string, value any) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed.Load() {
		return
	}

	// For simple comparison of equality
	if existing, ok := a.local[key]; ok && existing == value {
		return
	}
	a.local[key] = value

	if a.manager != nil && a.persist.Load() {
		a.dirty[key] = value
	}
}

// Flush writes unresolved dirty entries to BoltDB in a single batch operation.
func (a *DiagramCacheAdapter) Flush() error {
	if a.manager == nil {
		return nil
	}

	// Copy only dirty entries under lock, then release before I/O.
	a.mu.RLock()
	if len(a.dirty) == 0 {
		a.mu.RUnlock()
		return nil
	}
	dirtyCopy := make(map[string]any, len(a.dirty))
	maps.Copy(dirtyCopy, a.dirty)
	a.mu.RUnlock()

	slog.Info("Flushing diagram cache to BoltDB", "entries", len(dirtyCopy))

	if err := a.manager.BatchStoreSSR(dirtyCopy); err != nil {
		return err
	}

	// Success: clear dirty entries that were flushed
	a.mu.Lock()
	for k, v := range dirtyCopy {
		if current, ok := a.dirty[k]; ok && current == v {
			delete(a.dirty, k)
		}
	}
	a.mu.Unlock()

	return nil
}

// Merge stores a batch of values in the adapter and marks them dirty.
func (a *DiagramCacheAdapter) Merge(entries map[string]any) {
	for key, value := range entries {
		a.Set(key, value)
	}
}

// Load implements SSRMap for the markdown parser
func (a *DiagramCacheAdapter) Load(key string) (any, bool) {
	return a.Get(key)
}

// Store implements SSRMap for the markdown parser
func (a *DiagramCacheAdapter) Store(key string, value any) {
	a.Set(key, value)
}

// SetPersistenceEnabled controls whether Set() persists entries to disk.
func (a *DiagramCacheAdapter) SetPersistenceEnabled(enabled bool) {
	a.persist.Store(enabled)
}

// AsMap returns the local cache as a map (for compatibility)
func (a *DiagramCacheAdapter) AsMap() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]any, len(a.local))
	maps.Copy(result, a.local)
	return result
}

// Close marks the adapter as closed.
func (a *DiagramCacheAdapter) Close() error {
	a.closed.Store(true)
	return nil
}
