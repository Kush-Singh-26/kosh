package run

import "testing"

func TestShouldGeneratePWAIcons(t *testing.T) {
	tests := []struct {
		name     string
		force    bool
		hash     bool
		cache192 bool
		cache512 bool
		expected bool
	}{
		{name: "force always generates", force: true, hash: true, cache192: true, cache512: true, expected: true},
		{name: "all cache present skips", force: false, hash: true, cache192: true, cache512: true, expected: false},
		{name: "missing hash generates", force: false, hash: false, cache192: true, cache512: true, expected: true},
		{name: "missing 192 generates", force: false, hash: true, cache192: false, cache512: true, expected: true},
		{name: "missing 512 generates", force: false, hash: true, cache192: true, cache512: false, expected: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldGeneratePWAIcons(tc.force, tc.hash, tc.cache192, tc.cache512)
			if got != tc.expected {
				t.Fatalf("got %v want %v", got, tc.expected)
			}
		})
	}
}
