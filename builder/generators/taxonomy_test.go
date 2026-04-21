package generators

import (
"testing"

"github.com/Kush-Singh-26/kosh/builder/cache/core"
)

func TestBoundedTaxonomySocialCardWorkers(t *testing.T) {
	workers := BoundedTaxonomySocialCardWorkers()
	if workers < 1 {
		t.Fatalf("workers should be at least 1, got %d", workers)
	}
	if workers > maxTaxonomySocialCardWorkers {
		t.Fatalf("workers should be bounded to %d, got %d", maxTaxonomySocialCardWorkers, workers)
	}
}

func TestSocialCardHash_Stable(t *testing.T) {
	h1 := SocialCardHash("Title", "Description", nil)
	h2 := SocialCardHash("Title", "Description", nil)
	if h1 != h2 {
		t.Fatalf("hash should be deterministic")
	}
	if h1 != core.HashString("Title|Description") {
		t.Fatalf("hash should match cache hashing strategy")
	}
}
