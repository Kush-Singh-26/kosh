package cache

import (
	"testing"
)

func TestCacheService_SocialCardHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	path := "content/posts/test.md"
	expectedHash := "abc123"

	if err := service.SetSocialCardHash(path, expectedHash); err != nil {
		t.Fatalf("SetSocialCardHash failed: %v", err)
	}

	hash, err := service.GetSocialCardHash(path)
	if err != nil {
		t.Fatalf("GetSocialCardHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}

	hash, err = service.GetSocialCardHash("non-existent.md")
	if err == nil {
		t.Fatal("GetSocialCardHash should error for missing path")
	}

	if !IsCacheMiss(err) {
		t.Fatalf("Expected cache miss error, got %v", err)
	}

	if hash != "" {
		t.Errorf("Hash for non-existent path should be empty, got %q", hash)
	}
}

func TestCacheService_GraphHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	expectedHash := "graph-hash-123"

	if err := service.SetGraphHash(expectedHash); err != nil {
		t.Fatalf("SetGraphHash failed: %v", err)
	}

	hash, err := service.GetGraphHash()
	if err != nil {
		t.Fatalf("GetGraphHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}
}

func TestCacheService_WasmHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	expectedHash := "wasm-hash-456"

	if err := service.SetWasmHash(expectedHash); err != nil {
		t.Fatalf("SetWasmHash failed: %v", err)
	}

	hash, err := service.GetWasmHash()
	if err != nil {
		t.Fatalf("GetWasmHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}
}

func TestCacheService_IncrementBuildCount(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	stats, err := service.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	initialCount := stats.BuildCount

	count, err := service.IncrementBuildCount()
	if err != nil {
		t.Fatalf("IncrementBuildCount failed: %v", err)
	}
	if count == 0 {
		t.Errorf("Expected count > 0, got %d", count)
	}

	stats, err = service.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.BuildCount != initialCount+1 {
		t.Errorf("BuildCount = %d, want %d", stats.BuildCount, initialCount+1)
	}
}
