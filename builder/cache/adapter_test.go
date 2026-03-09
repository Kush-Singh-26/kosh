package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestDiagramCacheAdapter_ConcurrentSameKeyFlush(t *testing.T) {
	m, err := OpenWithTimeout(t.TempDir(), true, time.Second)
	if err != nil {
		t.Fatalf("failed to open cache manager: %v", err)
	}
	defer func() { _ = m.Close() }()

	a := NewDiagramCacheAdapter(m)
	defer func() { _ = a.Close() }()

	const workers = 12
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
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
