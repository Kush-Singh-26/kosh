package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchScenario(t *testing.T) {
	dir := t.TempDir()

	startWatcherWithConfig([]string{dir}, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	tmpFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	stopWatcher()

	t.Log("Watch scenario test passed")
}
