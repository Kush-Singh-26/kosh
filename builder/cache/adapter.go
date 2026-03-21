package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// writeRequest represents a request to write SSR data to cache
type writeRequest struct {
	key   string
	value any
}

// DiagramCacheAdapter provides a map[string]any interface backed by BoltDB
// This allows the existing markdown parser to work with the new cache system
type DiagramCacheAdapter struct {
	manager    *Manager
	local      map[string]any // In-memory buffer for current build
	dirty      map[string]any // Entries not yet durably persisted
	mu         sync.RWMutex
	pending    sync.WaitGroup    // Tracks pending async writes to prevent goroutine leaks
	closed     atomic.Bool       // Prevents new operations after Close() is called
	persist    atomic.Bool       // Controls whether writes are persisted to disk
	writeQueue chan writeRequest // Bounded queue for async writes
	workers    int               // Number of worker goroutines
	stopCh     chan struct{}     // Signal to stop workers
	closeOnce  sync.Once         // Ensures Close() is only called once
	writeGroup singleflight.Group
}

// NewDiagramCacheAdapter creates a new adapter with a bounded worker pool.
// Uses runtime.NumCPU() workers to limit concurrent async writes.
// Caller must call Start() to begin worker processing and Close() to shutdown.
func NewDiagramCacheAdapter(manager *Manager) *DiagramCacheAdapter {
	workers := max(runtime.NumCPU(), 2)

	a := &DiagramCacheAdapter{
		manager:    manager,
		local:      make(map[string]any),
		dirty:      make(map[string]any),
		writeQueue: make(chan writeRequest, 2048), // Large buffer to absorb processing spikes
		workers:    workers,
		stopCh:     make(chan struct{}),
	}
	a.persist.Store(true)

	// Workers are NOT started here - caller must explicitly call Start()
	// This makes the lifecycle explicit and testable
	return a
}

// Start begins the worker pool for async cache writes.
// Must be called after construction and before any Set() operations.
// Safe to call only once - subsequent calls are no-ops.
func (a *DiagramCacheAdapter) Start() {
	// Start worker pool
	for i := 0; i < a.workers; i++ {
		go a.writeWorker()
	}
}

func (a *DiagramCacheAdapter) persistSSRValue(key string, value any) error {
	if a.manager == nil {
		return nil
	}

	var bytes []byte
	var err error
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	default:
		bytes, err = json.Marshal(v)
		if err != nil {
			return err
		}
	}

	category := "d2"
	actualKey := key
	if parts := strings.SplitN(key, ":", 2); len(parts) == 2 {
		category = parts[0]
		actualKey = parts[1]
	}

	_, err, _ = a.writeGroup.Do(key, func() (any, error) {
		_, err := a.manager.StoreSSR(category, actualKey, bytes)
		if err == nil {
			a.clearDirtyIfUnchanged(key, value)
		}
		return nil, err
	})
	return err
}

// writeWorker processes write requests from the queue
func (a *DiagramCacheAdapter) writeWorker() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("writeWorker panic recovered", "panic", r)
		}
	}()

	for {
		select {
		case req := <-a.writeQueue:
			if err := a.persistSSRValue(req.key, req.value); err != nil {
				// Log error but don't fail - the data is still in local cache
				slog.Warn("Failed to store SSR cache", "key", req.key, "error", err)
			}
			a.pending.Done()
		case <-a.stopCh:
			return
		}
	}
}

// Get retrieves a cached diagram
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

// Set stores a diagram in the cache
// Uses bounded worker pool to prevent goroutine explosion with many diagrams
func (a *DiagramCacheAdapter) Set(key string, value any) {
	a.mu.Lock()
	if a.closed.Load() {
		a.mu.Unlock()
		return
	}

	// For simple comparison of equality
	// Note: this might not work perfectly for structs but good enough for now
	if existing, ok := a.local[key]; ok && existing == value {
		a.mu.Unlock()
		return
	}
	a.local[key] = value
	managerAvailable := a.manager != nil
	if managerAvailable && a.persist.Load() {
		a.dirty[key] = value
	}
	a.mu.Unlock()

	// Also store in BoltDB if manager is available using worker pool.
	// Never block parse workers on cache I/O; unresolved entries remain dirty and are flushed later.
	if managerAvailable && a.persist.Load() {
		a.pending.Add(1)
		select {
		case a.writeQueue <- writeRequest{key: key, value: value}:
		default:
			a.pending.Done()
		}
	}
}

// Flush writes unresolved dirty entries to BoltDB.
func (a *DiagramCacheAdapter) Flush() error {
	if a.manager == nil {
		return nil
	}

	a.pending.Wait()

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

	// Parallelize dirty entry persistence with bounded concurrency
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())
	for k, v := range dirtyCopy {
		key, val := k, v
		g.Go(func() error {
			return a.persistSSRValue(key, val)
		})
	}
	return g.Wait()
}

// Merge stores a batch of values in the adapter and schedules persistence.
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

// Close waits for all pending async operations to complete and closes the adapter.
// This should be called during shutdown to prevent goroutine leaks.
// Safe to call multiple times - uses sync.Once to prevent double-close panic.
func (a *DiagramCacheAdapter) Close() error {
	a.closed.Store(true)

	// Wait for all pending writes to complete
	a.pending.Wait()

	// Signal workers to stop (only once, protected by sync.Once)
	a.closeOnce.Do(func() {
		close(a.stopCh)
	})

	// Flush is handled by SaveCaches() in the normal lifecycle.
	return nil
}

func (a *DiagramCacheAdapter) clearDirtyIfUnchanged(key string, value any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if current, ok := a.dirty[key]; ok && current == value {
		delete(a.dirty, key)
	}
}
