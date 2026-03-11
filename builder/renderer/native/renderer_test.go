package native

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRenderer_RenderD2(t *testing.T) {
	r := New(WithWorkers(1))
	defer r.Close()

	ctx := context.Background()
	code := "x -> y: hello world"
	svg, err := r.RenderD2(ctx, code, 0)
	if err != nil {
		t.Fatalf("RenderD2 failed: %v", err)
	}

	if !strings.Contains(svg, "<svg") {
		t.Error("RenderD2 result does not contain <svg tag")
	}
	if !strings.Contains(svg, "hello world") {
		t.Error("RenderD2 result does not contain label text")
	}
}

func TestRenderer_RenderMath(t *testing.T) {
	r := New(WithWorkers(1))
	defer r.Close()

	ctx := context.Background()
	latex := "E = mc^2"
	html, err := r.RenderMath(ctx, latex, true)
	if err != nil {
		t.Fatalf("RenderMath failed: %v", err)
	}

	if !strings.Contains(html, "katex") {
		t.Error("RenderMath result does not contain 'katex' class")
	}
	if !strings.Contains(html, "m") || !strings.Contains(html, "c") {
		t.Error("RenderMath result does not contain 'm' or 'c'")
	}
}

func TestRenderer_RenderAllMath(t *testing.T) {
	r := New(WithWorkers(2))
	defer r.Close()

	ctx := context.Background()
	expressions := []MathExpression{
		{LaTeX: "a^2 + b^2 = c^2", DisplayMode: true, Hash: "hash1"},
		{LaTeX: "\\sum_{i=1}^n i", DisplayMode: false, Hash: "hash2"},
	}

	cache := make(map[string]string)
	results, err := r.RenderAllMath(ctx, expressions, cache)
	if err != nil {
		t.Fatalf("RenderAllMath failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if !strings.Contains(results["hash1"], "katex") || !strings.Contains(results["hash1"], "a") {
		t.Errorf("Result for hash1 looks wrong: %s", results["hash1"])
	}

	// Test cache hit
	cache["hash1"] = "cached-result"
	results2, err := r.RenderAllMath(ctx, expressions, cache)
	if err != nil {
		t.Fatalf("RenderAllMath with cache failed: %v", err)
	}
	if results2["hash1"] != "cached-result" {
		t.Error("Cache hit failed for RenderAllMath")
	}
}

func TestRenderer_ConcurrentInitialization(t *testing.T) {
	r := New(WithWorkers(2))

	var wg sync.WaitGroup

	for range 10 {
		wg.Go(func() {
			r.ensureInitialized()
		})
	}

	wg.Wait()

	r.ensureInitialized()
	t.Log("Concurrent initialization test passed")
}

func TestRenderer_PoolChannels(t *testing.T) {
	r := New(WithWorkers(2))

	r.ensureInitialized()

	select {
	case instance := <-r.pool:
		r.pool <- instance
	case <-time.After(5 * time.Second): // Increase timeout for WASM cold start
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
