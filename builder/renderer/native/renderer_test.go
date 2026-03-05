package native

import (
	"sync"
	"testing"
	"time"
)

func TestRenderer_ConcurrentInitialization(t *testing.T) {
	r := New(WithWorkers(2))

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ensureInitialized()
		}()
	}

	wg.Wait()

	r.ensureInitialized()

	if r.katexProg == nil {
		t.Error("katexProg should be initialized after ensureInitialized")
	}

	t.Log("Concurrent initialization test passed")
}

func TestRenderer_EnsureInitialized_Once(t *testing.T) {
	r := New(WithWorkers(1))

	r.ensureInitialized()
	prog1 := r.katexProg

	r.ensureInitialized()
	prog2 := r.katexProg

	if prog1 != prog2 {
		t.Error("ensureInitialized should only run once")
	}

	t.Log("ensureInitialized once test passed")
}

func TestRenderer_KatexProgAvailable(t *testing.T) {
	r := New(WithWorkers(1))

	r.ensureInitialized()

	if r.katexProg == nil {
		t.Error("katexProg should not be nil after initialization")
	}

	t.Log("katexProg available test passed")
}

func TestRenderer_PoolChannels(t *testing.T) {
	r := New(WithWorkers(2))

	r.ensureInitialized()

	select {
	case instance := <-r.pool:
		r.pool <- instance
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for instance from pool")
	}

	t.Log("Pool channels test passed")
}

func TestRenderer_Close(t *testing.T) {
	r := New(WithWorkers(2))

	// Close before initialization
	if err := r.Close(); err != nil {
		t.Errorf("Close failed before initialization: %v", err)
	}

	// Close after initialization
	r2 := New(WithWorkers(2))
	r2.ensureInitialized()
	if err := r2.Close(); err != nil {
		t.Errorf("Close failed after initialization: %v", err)
	}

	// Double close
	if err := r2.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}

	t.Log("Close tests passed")
}

func TestRenderer_Close_Race(t *testing.T) {
	r := New(WithWorkers(4))

	// Trigger lazy initialization and immediate close
	go r.ensureInitialized()
	time.Sleep(10 * time.Millisecond) // Small delay to let init start
	if err := r.Close(); err != nil {
		t.Errorf("Close failed during initialization: %v", err)
	}

	t.Log("Close race test passed")
}
