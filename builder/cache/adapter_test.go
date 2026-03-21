package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestDiagramCacheAdapter_ConcurrentSameKeyFlush(t *testing.T) {
	m, err := OpenWithTimeout(t.TempDir(), true, time.Second)
	if err != nil {
		t.Fatalf("failed to open cache manager: %v", err)
	}
	defer func() { _ = m.Close() }()

	a := NewDiagramCacheAdapter(m)
	a.Start() // Explicit lifecycle start
	defer func() { _ = a.Close() }()

	const workers = 12
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a.Set("same-key", fmt.Sprintf("value-%d", i%2))
		}(i)
	}
	wg.Wait()

	if err := a.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	artifact, err := m.GetSSRArtifact("d2", "same-key")
	if err != nil || artifact == nil {
		t.Fatalf("expected SSR artifact after flush, err=%v", err)
	}
}

func TestDiagramCacheAdapter_SSRThemePair(t *testing.T) {
	m, err := OpenWithTimeout(t.TempDir(), true, time.Second)
	if err != nil {
		t.Fatalf("failed to open cache manager: %v", err)
	}
	defer func() { _ = m.Close() }()

	a := NewDiagramCacheAdapter(m)
	a.Start()
	defer func() { _ = a.Close() }()

	key := "d2-diagram-hash"
	pair := models.SSRThemePair{
		Light: "light-svg-content",
		Dark:  "dark-svg-content",
	}

	a.Set(key, pair)

	// Test GetLocal (in-memory)
	if val, ok := a.GetLocal(key); !ok {
		t.Fatal("expected to find pair in local cache")
	} else if p, ok := val.(models.SSRThemePair); !ok || p != pair {
		t.Fatalf("expected %v, got %v", pair, val)
	}

	// Flush to BoltDB
	if err := a.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	// Create a fresh adapter to test BoltDB retrieval
	a2 := NewDiagramCacheAdapter(m)
	defer func() { _ = a2.Close() }()

	if val, ok := a2.Get(key); !ok {
		t.Fatal("expected to find pair in BoltDB via fresh adapter")
	} else if p, ok := val.(models.SSRThemePair); !ok || p != pair {
		t.Fatalf("expected %v after BoltDB retrieval, got %v", pair, val)
	}
}
