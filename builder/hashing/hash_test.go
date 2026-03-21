package hashing

import (
	"errors"
	"testing"
)

func TestGetBodyHash(t *testing.T) {
	tests := []struct {
		name    string
		source  []byte
		wantLen int
	}{
		{
			name:    "markdown with frontmatter",
			source:  []byte("---\ntitle: Test\n---\nThis is the body content."),
			wantLen: 32,
		},
		{
			name:    "markdown without frontmatter",
			source:  []byte("Just plain markdown content."),
			wantLen: 32,
		},
		{
			name:    "empty content",
			source:  []byte(""),
			wantLen: 32,
		},
		{
			name:    "frontmatter only",
			source:  []byte("---\ntitle: Test\n---\n"),
			wantLen: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := GetBodyHash(tt.source)
			if len(hash) != tt.wantLen {
				t.Errorf("GetBodyHash() returned hash of length %d, want %d", len(hash), tt.wantLen)
			}
		})
	}
}

func TestGetBodyHashDeterministic(t *testing.T) {
	source := []byte("---\ntitle: Test\n---\nBody content here.")

	hash1 := GetBodyHash(source)
	hash2 := GetBodyHash(source)

	if hash1 != hash2 {
		t.Errorf("GetBodyHash() not deterministic: %s != %s", hash1, hash2)
	}
}

func TestGetBodyHashDifferentContent(t *testing.T) {
	source1 := []byte("---\ntitle: Test\n---\nContent A")
	source2 := []byte("---\ntitle: Test\n---\nContent B")

	hash1 := GetBodyHash(source1)
	hash2 := GetBodyHash(source2)

	if hash1 == hash2 {
		t.Error("Different body content should produce different hashes")
	}
}

func TestGetBodyHashIgnoresFrontmatter(t *testing.T) {
	source1 := []byte("---\ntitle: First\n---\nSame body")
	source2 := []byte("---\ntitle: Second\n---\nSame body")

	hash1 := GetBodyHash(source1)
	hash2 := GetBodyHash(source2)

	if hash1 != hash2 {
		t.Error("Same body with different frontmatter should produce same body hash")
	}
}

func TestGetFrontmatterHash(t *testing.T) {
	tests := []struct {
		name     string
		metaData map[string]any
		wantErr  bool
	}{
		{
			name: "complete metadata",
			metaData: map[string]any{
				"title":       "Test Post",
				"description": "A test description",
				"date":        "2026-02-12",
				"tags":        []any{"go", "testing", "ssg"},
				"pinned":      true,
			},
			wantErr: false,
		},
		{
			name:     "empty metadata",
			metaData: map[string]any{},
			wantErr:  false,
		},
		{
			name: "only title",
			metaData: map[string]any{
				"title": "Just a Title",
			},
			wantErr: false,
		},
		{
			name: "tags unsorted",
			metaData: map[string]any{
				"title": "Post with Tags",
				"tags":  []any{"zebra", "alpha", "beta"},
			},
			wantErr: false,
		},
		{
			name: "pinned false",
			metaData: map[string]any{
				"title":  "Not Pinned",
				"pinned": false,
			},
			wantErr: false,
		},
		{
			name: "with special characters",
			metaData: map[string]any{
				"title":       "Post with <html> & \"quotes\"",
				"description": "Description with unicode: ñ, 中文, 🎉",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := GetFrontmatterHash(tt.metaData)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFrontmatterHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if hash == "" {
				t.Error("GetFrontmatterHash() returned empty hash")
			}
			if len(hash) != 32 { // XXH3-128 hex string length
				t.Errorf("GetFrontmatterHash() returned hash of length %d, want 32", len(hash))
			}
		})
	}
}

func TestGetFrontmatterHashDeterministic(t *testing.T) {
	metaData := map[string]any{
		"title":       "Test Post",
		"description": "A test description",
		"date":        "2026-02-12",
		"tags":        []any{"go", "testing"},
		"pinned":      true,
	}

	hash1, err := GetFrontmatterHash(metaData)
	if err != nil {
		t.Fatalf("First hash computation failed: %v", err)
	}

	hash2, err := GetFrontmatterHash(metaData)
	if err != nil {
		t.Fatalf("Second hash computation failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("GetFrontmatterHash() not deterministic: %s != %s", hash1, hash2)
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "valid yaml",
			data:    []byte("title: Test"),
			wantErr: nil,
		},
		{
			name:    "empty data",
			data:    []byte(""),
			wantErr: ErrEmptyData,
		},
		{
			name:    "invalid yaml",
			data:    []byte("title: [unclosed"),
			wantErr: errors.New("yaml: line 1: did not find expected ',' or ']'"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFrontmatter(tt.data)
			if tt.wantErr != nil {
				if err == nil || err.Error() != tt.wantErr.Error() {
					t.Errorf("ParseFrontmatter() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ParseFrontmatter() unexpected error = %v", err)
			}
		})
	}
}
