package cache

import (
	"log"
	"runtime"
	"sync"
)

// writeRequest represents a request to write SSR data to cache
type writeRequest struct {
	key   string
	value string
}

// DiagramCacheAdapter provides a map[string]string interface backed by BoltDB
// This allows the existing markdown parser to work with the new cache system
type DiagramCacheAdapter struct {
	manager    *Manager
	local      map[string]string // In-memory buffer for current build
	mu         sync.RWMutex
	pending    sync.WaitGroup    // Tracks pending async writes to prevent goroutine leaks
	closed     bool              // Prevents new operations after Close() is called
	writeQueue chan writeRequest // Bounded queue for async writes
	workers    int               // Number of worker goroutines
	stopCh     chan struct{}     // Signal to stop workers
	closeOnce  sync.Once         // Ensures Close() is only called once
}

// NewDiagramCacheAdapter creates a new adapter with a bounded worker pool
// Uses runtime.NumCPU() workers to limit concurrent async writes
func NewDiagramCacheAdapter(manager *Manager) *DiagramCacheAdapter {
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}

	a := &DiagramCacheAdapter{
		manager:    manager,
		local:      make(map[string]string),
		writeQueue: make(chan writeRequest, workers*4), // Buffered queue
		workers:    workers,
		stopCh:     make(chan struct{}),
	}

	// Start worker pool
	for i := 0; i < workers; i++ {
		go a.writeWorker()
	}

	return a
}

// writeWorker processes write requests from the queue
func (a *DiagramCacheAdapter) writeWorker() {
	for {
		select {
		case req := <-a.writeQueue:
			if _, err := a.manager.StoreSSR("d2", req.key, []byte(req.value)); err != nil {
				// Log error but don't fail - the data is still in local cache
				log.Printf("Failed to store SSR cache for key %s: %v", req.key, err)
			}
			a.pending.Done()
		case <-a.stopCh:
			return
		}
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
// Uses bounded worker pool to prevent goroutine explosion with many diagrams
func (a *DiagramCacheAdapter) Set(key string, value string) {
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return
	}
	a.mu.RUnlock()

	a.mu.Lock()
	a.local[key] = value
	a.mu.Unlock()

	// Also store in BoltDB if manager is available using worker pool
	if a.manager != nil {
		select {
		case a.writeQueue <- writeRequest{key: key, value: value}:
			// Successfully queued - worker will call Done()
			a.pending.Add(1)
		default:
			// Queue full, process synchronously to avoid blocking
			if _, err := a.manager.StoreSSR("d2", key, []byte(value)); err != nil {
				log.Printf("Failed to store SSR cache for key %s: %v", key, err)
			}
			// Note: pending.Add(1) is NOT called for synchronous path
		}
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

// Close waits for all pending async operations to complete and closes the adapter.
// This should be called during shutdown to prevent goroutine leaks.
// Safe to call multiple times - uses sync.Once to prevent double-close panic.
func (a *DiagramCacheAdapter) Close() error {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()

	// Wait for all pending writes to complete
	a.pending.Wait()

	// Signal workers to stop (only once, protected by sync.Once)
	a.closeOnce.Do(func() {
		close(a.stopCh)
	})

	// Flush any remaining local entries
	return a.Flush()
}
