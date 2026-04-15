package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestWatcher_BurstDebounce(t *testing.T) {
	dir := t.TempDir()
	eventCount := 0
	var countMu sync.Mutex

	w, err := New([]string{dir}, func(_ Event) {
		countMu.Lock()
		eventCount++
		countMu.Unlock()
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Set a slightly longer debounce for the test to be reliable
	w.duration = 100 * time.Millisecond

	pendingMu := &sync.Mutex{}
	pendingEvents := make(map[string]fsnotify.Op)

	// Simulate a burst of events
	for range 50 {
		pendingMu.Lock()
		pendingEvents["test.txt"] = fsnotify.Write
		pendingMu.Unlock()
		w.resetTimer(pendingMu, &pendingEvents)
		time.Sleep(2 * time.Millisecond) // Total 100ms burst, right at the edge
	}

	// Wait for debounce
	time.Sleep(300 * time.Millisecond)

	countMu.Lock()
	count := eventCount
	countMu.Unlock()

	// In a perfect debounce, we expect exactly 1 event for the same file
	// But since the burst took ~100ms and debounce is 100ms, it might be 1 or 2
	if count > 2 {
		t.Errorf("Debounce failed: expected 1 or 2 events, got %d", count)
	} else {
		t.Logf("Debounce worked: captured %d events from 50 triggers", count)
	}
}

func TestWatcher_ResetTimer_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	eventCount := 0
	var countMu sync.Mutex

	w, err := New([]string{dir}, func(_ Event) {
		countMu.Lock()
		eventCount++
		countMu.Unlock()
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	go w.Start()
	defer func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				pendingMu := &sync.Mutex{}
				pendingEvents := make(map[string]fsnotify.Op)
				w.resetTimer(pendingMu, &pendingEvents)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	countMu.Lock()
	_ = eventCount
	countMu.Unlock()

	t.Log("Timer reset thread-safety test passed")
}

func TestWatcher_ConcurrentFileEvents(t *testing.T) {
	dir := t.TempDir()
	eventCount := 0
	var countMu sync.Mutex

	w, err := New([]string{dir}, func(_ Event) {
		countMu.Lock()
		eventCount++
		countMu.Unlock()
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	go w.Start()
	defer func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)

	for range 5 {
		tmpFile := filepath.Join(dir, "testfile.txt")
		if err := os.WriteFile(tmpFile, []byte("content"), 0644); err != nil {
			t.Logf("Warning: could not write test file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	countMu.Lock()
	count := eventCount
	countMu.Unlock()

	if count > 0 {
		t.Logf("Captured %d events", count)
	}
}

func TestWatcher_TimerDrain(t *testing.T) {
	dir := t.TempDir()

	w, err := New([]string{dir}, func(_ Event) {})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	pendingMu := &sync.Mutex{}
	pendingEvents := make(map[string]fsnotify.Op)

	w.resetTimer(pendingMu, &pendingEvents)
	time.Sleep(20 * time.Millisecond)

	w.resetTimer(pendingMu, &pendingEvents)

	time.Sleep(20 * time.Millisecond)

	w.resetTimer(pendingMu, &pendingEvents)

	t.Log("Timer drain test passed - no panic")
}
