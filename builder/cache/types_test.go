package cache

import (
	"bytes"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestHashContent(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		empty bool
	}{
		{
			name:  "simple string",
			data:  []byte("hello world"),
			empty: false,
		},
		{
			name:  "empty data",
			data:  []byte{},
			empty: false, // hash of empty data is not empty
		},
		{
			name:  "binary data",
			data:  []byte{0x00, 0x01, 0x02, 0x03},
			empty: false,
		},
		{
			name:  "unicode text",
			data:  []byte("Hello, 世界! 🌍"),
			empty: false,
		},
		{
			name:  "long content",
			data:  bytes.Repeat([]byte("a"), 10000),
			empty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashContent(tt.data)
			if hash == "" && !tt.empty {
				t.Error("HashContent() returned empty string")
			}
			// BLAKE3 produces 32 bytes = 64 hex characters
			if len(hash) != 64 {
				t.Errorf("HashContent() returned hash of length %d, want 64", len(hash))
			}
		})
	}
}

func TestHashContentDeterministic(t *testing.T) {
	data := []byte("test data for hashing")

	hash1 := HashContent(data)
	hash2 := HashContent(data)

	if hash1 != hash2 {
		t.Errorf("HashContent() not deterministic: %s != %s", hash1, hash2)
	}
}

func TestHashContentDifferentData(t *testing.T) {
	hash1 := HashContent([]byte("data1"))
	hash2 := HashContent([]byte("data2"))

	if hash1 == hash2 {
		t.Error("Different data should produce different hashes")
	}
}

func TestHashString(t *testing.T) {
	tests := []struct {
		name string
		s    string
	}{
		{
			name: "simple string",
			s:    "hello",
		},
		{
			name: "empty string",
			s:    "",
		},
		{
			name: "unicode string",
			s:    "Hello, 世界!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashString(tt.s)
			if hash == "" && tt.s != "" {
				t.Error("HashString() returned empty string for non-empty input")
			}
			if len(hash) != 64 {
				t.Errorf("HashString() returned hash of length %d, want 64", len(hash))
			}
		})
	}
}

func TestHashStringConsistency(t *testing.T) {
	s := "consistent string"

	hash1 := HashString(s)
	hash2 := HashContent([]byte(s))

	if hash1 != hash2 {
		t.Error("HashString and HashContent should produce same result for same data")
	}
}

func TestGeneratePostID(t *testing.T) {
	tests := []struct {
		name           string
		uuid           string
		normalizedPath string
		wantEmpty      bool
	}{
		{
			name:           "with uuid",
			uuid:           "550e8400-e29b-41d4-a716-446655440000",
			normalizedPath: "/path/to/post",
			wantEmpty:      false,
		},
		{
			name:           "without uuid uses path",
			uuid:           "",
			normalizedPath: "/path/to/post",
			wantEmpty:      false,
		},
		{
			name:           "empty both",
			uuid:           "",
			normalizedPath: "",
			wantEmpty:      false, // hash of empty string is not empty
		},
		{
			name:           "uuid takes precedence",
			uuid:           "uuid-value",
			normalizedPath: "path-value",
			wantEmpty:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := GeneratePostID(tt.uuid, tt.normalizedPath)
			if id == "" && !tt.wantEmpty {
				t.Error("GeneratePostID() returned empty string")
			}
			if len(id) != 64 {
				t.Errorf("GeneratePostID() returned ID of length %d, want 64", len(id))
			}
		})
	}
}

func TestGeneratePostIDDeterministic(t *testing.T) {
	uuid := "test-uuid-123"
	path := "/test/path"

	id1 := GeneratePostID(uuid, path)
	id2 := GeneratePostID(uuid, path)

	if id1 != id2 {
		t.Errorf("GeneratePostID() not deterministic: %s != %s", id1, id2)
	}
}

func TestGeneratePostIDUUIDPriority(t *testing.T) {
	uuid := "uuid-value"
	path := "path-value"

	idWithUUID := GeneratePostID(uuid, path)
	idWithoutUUID := GeneratePostID("", path)

	if idWithUUID == idWithoutUUID {
		t.Error("UUID should take precedence over path")
	}
}

func TestEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "PostMeta",
			data: &PostMeta{
				PostID:      "test-id",
				Title:       "Test Title",
				Description: "Test Description",
				Tags:        []string{"go", "testing"},
				Date:        time.Now(),
				WordCount:   100,
				ReadingTime: 5,
			},
		},
		{
			name: "SearchRecord",
			data: &SearchRecord{
				Title:           "Test",
				NormalizedTitle: "test",
				Tokens:          []string{"test", "record"},
				BM25Data:        map[string]int{"test": 1, "record": 2},
				DocLen:          10,
				Content:         "test content",
			},
		},
		{
			name: "CacheStats",
			data: &CacheStats{
				TotalPosts:    10,
				TotalSSR:      5,
				StoreBytes:    1024,
				BuildCount:    3,
				SchemaVersion: 1,
			},
		},
		{
			name: "Dependencies",
			data: &Dependencies{
				Templates: []string{"layout.html", "post.html"},
				Includes:  []string{"header.html"},
				Tags:      []string{"go", "ssg"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if len(encoded) == 0 {
				t.Error("Encode() returned empty bytes")
			}

			// Decode based on type
			switch v := tt.data.(type) {
			case *PostMeta:
				var decoded PostMeta
				if err := Decode(encoded, &decoded); err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if decoded.PostID != v.PostID {
					t.Errorf("PostID mismatch: got %q, want %q", decoded.PostID, v.PostID)
				}
				if decoded.Title != v.Title {
					t.Errorf("Title mismatch: got %q, want %q", decoded.Title, v.Title)
				}
			case *SearchRecord:
				var decoded SearchRecord
				if err := Decode(encoded, &decoded); err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if decoded.Title != v.Title {
					t.Errorf("Title mismatch: got %q, want %q", decoded.Title, v.Title)
				}
				if decoded.DocLen != v.DocLen {
					t.Errorf("DocLen mismatch: got %d, want %d", decoded.DocLen, v.DocLen)
				}
			case *CacheStats:
				var decoded CacheStats
				if err := Decode(encoded, &decoded); err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if decoded.TotalPosts != v.TotalPosts {
					t.Errorf("TotalPosts mismatch: got %d, want %d", decoded.TotalPosts, v.TotalPosts)
				}
			case *Dependencies:
				var decoded Dependencies
				if err := Decode(encoded, &decoded); err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if len(decoded.Templates) != len(v.Templates) {
					t.Errorf("Templates length mismatch: got %d, want %d", len(decoded.Templates), len(v.Templates))
				}
			}
		})
	}
}

func TestEncodeDecodeComplex(t *testing.T) {
	original := &PostMeta{
		PostID:         "complex-id",
		Path:           "/posts/complex.md",
		ContentHash:    HashString("content"),
		TemplateHash:   HashString("template"),
		SSRInputHashes: []string{"hash1", "hash2"},
		Title:          "Complex Post",
		Date:           time.Now().UTC(),
		Tags:           []string{"go", "testing", "cache"},
		WordCount:      1500,
		ReadingTime:    8,
		Description:    "A complex post for testing",
		Link:           "/posts/complex",
		Weight:         10,
		Pinned:         true,
		Draft:          false,
		Meta:           map[string]interface{}{"author": "test", "category": "tech"},
		TOC: []models.TOCEntry{
			{ID: "intro", Text: "Introduction", Level: 1},
			{ID: "body", Text: "Body", Level: 2},
		},
		Version: "v1.0",
	}

	encoded, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	var decoded PostMeta
	if err := Decode(encoded, &decoded); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	// Verify all fields
	if decoded.PostID != original.PostID {
		t.Errorf("PostID mismatch")
	}
	if decoded.Title != original.Title {
		t.Errorf("Title mismatch")
	}
	if decoded.WordCount != original.WordCount {
		t.Errorf("WordCount mismatch")
	}
	if decoded.Pinned != original.Pinned {
		t.Errorf("Pinned mismatch")
	}
	if len(decoded.Tags) != len(original.Tags) {
		t.Errorf("Tags length mismatch")
	}
	if len(decoded.TOC) != len(original.TOC) {
		t.Errorf("TOC length mismatch")
	}
}

func TestConstants(t *testing.T) {
	// Test that constants have expected values
	// These constants are now in builder/utils/constants.go
	// and should be tested there or imported from there.
	// Removing this test as it was testing constants defined in this package
	// which have been moved.
}

func TestCompressionType(t *testing.T) {
	// Test enum values
	if CompressionNone != 0 {
		t.Errorf("CompressionNone = %d, want 0", CompressionNone)
	}
	if CompressionZstdFast != 1 {
		t.Errorf("CompressionZstdFast = %d, want 1", CompressionZstdFast)
	}
	if CompressionZstdLevel3 != 2 {
		t.Errorf("CompressionZstdLevel3 = %d, want 2", CompressionZstdLevel3)
	}
}
