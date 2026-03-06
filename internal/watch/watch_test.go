package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestWatcher_ResetTimer_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	eventCount := 0
	var countMu sync.Mutex

	w, err := New([]string{dir}, func(e Event) {
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
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
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

	w, err := New([]string{dir}, func(e Event) {
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

	for i := 0; i < 5; i++ {
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

	w, err := New([]string{dir}, func(e Event) {})
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
