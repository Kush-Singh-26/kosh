package cache

import (
	"context"
	"encoding/json"
	"fmt"
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
	mutex   sync.RWMutex // protects local and dirty
	closed  atomic.Bool  // Prevents new operations after Close() is called
	persist atomic.Bool  // Controls whether writes are persisted to disk
}

// NewDiagramCacheAdapter creates a new adapter.
func NewDiagramCacheAdapter(manager *Manager) *DiagramCacheAdapter {
	adapter := &DiagramCacheAdapter{
		manager: manager,
		local:   make(map[string]any),
		dirty:   make(map[string]any),
	}
	adapter.persist.Store(true)
	return adapter
}

// Start is a no-op kept for interface compatibility.
func (adapter *DiagramCacheAdapter) Start() {}

// Get retrieves a cached diagram.
func (adapter *DiagramCacheAdapter) Get(key string) (any, bool) {
	adapter.mutex.RLock()
	if value, ok := adapter.local[key]; ok {
		adapter.mutex.RUnlock()
		return value, true
	}
	adapter.mutex.RUnlock()

	// Try to get from BoltDB if manager is available
	if adapter.manager != nil {
		category := "d2"
		actualKey := key
		if parts := strings.SplitN(key, ":", 2); len(parts) == 2 {
			category = parts[0]
			actualKey = parts[1]
		}
		artifact, err := adapter.manager.GetSSRArtifact(category, actualKey)
		if err == nil && artifact != nil {
			content, err := adapter.manager.GetSSRContent(category, artifact)
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

				adapter.mutex.Lock()
				adapter.local[key] = result
				adapter.mutex.Unlock()
				return result, true
			}
		}
	}

	return nil, false
}

// GetLocal retrieves a cached diagram from in-memory state only.
// It avoids BoltDB lookups and is intended for hot-path render checks.
func (adapter *DiagramCacheAdapter) GetLocal(key string) (any, bool) {
	adapter.mutex.RLock()
	defer adapter.mutex.RUnlock()
	value, ok := adapter.local[key]
	return value, ok
}

// Set stores a diagram in the memory cache and marks it dirty for later flushing.
func (adapter *DiagramCacheAdapter) Set(key string, value any) {
	adapter.mutex.Lock()
	defer adapter.mutex.Unlock()

	if adapter.closed.Load() {
		return
	}

	// For simple comparison of equality
	if existing, ok := adapter.local[key]; ok && existing == value {
		return
	}
	adapter.local[key] = value

	if adapter.manager != nil && adapter.persist.Load() {
		adapter.dirty[key] = value
	}
}

// Flush writes unresolved dirty entries to BoltDB in a single batch operation.
func (adapter *DiagramCacheAdapter) Flush(ctx context.Context) error {
	if adapter.manager == nil {
		return nil
	}

	// Copy only dirty entries under lock, then release before I/O.
	adapter.mutex.RLock()
	if len(adapter.dirty) == 0 {
		adapter.mutex.RUnlock()
		return nil
	}
	dirtyCopy := make(map[string]any, len(adapter.dirty))
	maps.Copy(dirtyCopy, adapter.dirty)
	adapter.mutex.RUnlock()

	slog.Info("Flushing diagram cache to BoltDB", "entries", len(dirtyCopy))

	if err := adapter.manager.BatchStoreSSR(ctx, dirtyCopy); err != nil {
		return err
	}

	// Success: clear dirty entries that were flushed
	adapter.mutex.Lock()
	for k, v := range dirtyCopy {
		if current, ok := adapter.dirty[k]; ok && current == v {
			delete(adapter.dirty, k)
		}
	}
	adapter.mutex.Unlock()

	return nil
}

// Merge stores a batch of values in the adapter and marks them dirty.
// Entry values are SSR payloads: string (HTML/SVG) or models.SSRThemePair.
func (adapter *DiagramCacheAdapter) Merge(entries map[string]any) {
	for key, value := range entries {
		adapter.Set(key, value)
	}
}

// Load implements SSRMap for the markdown parser
func (adapter *DiagramCacheAdapter) Load(key string) (any, bool) {
	return adapter.Get(key)
}

// Store implements SSRMap for the markdown parser
func (adapter *DiagramCacheAdapter) Store(key string, value any) {
	adapter.Set(key, value)
}

// SetPersistenceEnabled controls whether Set() persists entries to disk.
func (adapter *DiagramCacheAdapter) SetPersistenceEnabled(enabled bool) {
	adapter.persist.Store(enabled)
}

// AsMap returns the local cache as a map (for compatibility)
func (adapter *DiagramCacheAdapter) AsMap() map[string]any {
	adapter.mutex.RLock()
	defer adapter.mutex.RUnlock()

	result := make(map[string]any, len(adapter.local))
	maps.Copy(result, adapter.local)
	return result
}

// Close marks the adapter as closed.
func (adapter *DiagramCacheAdapter) Close() error {
	adapter.closed.Store(true)
	return nil
}

// FragmentCacheAdapter provides a write-buffered FragmentCache implementation.
type FragmentCacheAdapter struct {
	manager *Manager
	dirty   map[string][]byte
	mutex   sync.Mutex
}

// NewFragmentCacheAdapter creates a new adapter.
func NewFragmentCacheAdapter(manager *Manager) *FragmentCacheAdapter {
	return &FragmentCacheAdapter{
		manager: manager,
		dirty:   make(map[string][]byte),
	}
}

// GetFragment retrieves a fragment from BoltDB.
func (adapter *FragmentCacheAdapter) GetFragment(key string) ([]byte, error) {
	if adapter == nil || adapter.manager == nil {
		return nil, fmt.Errorf("no cache manager available")
	}
	return adapter.manager.GetFragment(key)
}

// StoreFragment buffers a fragment for later flushing.
func (adapter *FragmentCacheAdapter) StoreFragment(key string, data []byte) error {
	if adapter == nil {
		return nil
	}
	adapter.mutex.Lock()
	defer adapter.mutex.Unlock()
	adapter.dirty[key] = data
	return nil
}

// Flush writes all dirty fragments to BoltDB in a single transaction.
func (adapter *FragmentCacheAdapter) Flush(ctx context.Context) error {
	if adapter == nil || adapter.manager == nil || len(adapter.dirty) == 0 {
		return nil
	}

	adapter.mutex.Lock()
	dirty := adapter.dirty
	adapter.dirty = make(map[string][]byte)
	adapter.mutex.Unlock()

	return adapter.manager.BatchStoreFragments(ctx, dirty)
}
