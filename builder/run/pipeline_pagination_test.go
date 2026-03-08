package run

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
)

func TestBoundedTagSocialCardWorkers(t *testing.T) {
	workers := boundedTagSocialCardWorkers()
	if workers < 1 {
		t.Fatalf("workers should be at least 1, got %d", workers)
	}
	if workers > maxTagSocialCardWorkers {
		t.Fatalf("workers should be bounded to %d, got %d", maxTagSocialCardWorkers, workers)
	}
}

func TestSocialCardHash_Stable(t *testing.T) {
	h1 := socialCardHash("Title", "Description")
	h2 := socialCardHash("Title", "Description")
	if h1 != h2 {
		t.Fatalf("hash should be deterministic")
	}
	if h1 != cache.HashString("Title|Description") {
		t.Fatalf("hash should match cache hashing strategy")
	}
}
