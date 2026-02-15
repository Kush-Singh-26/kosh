package utils

import (
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGetFrontmatterHash(t *testing.T) {
	tests := []struct {
		name     string
		metaData map[string]interface{}
		wantErr  bool
	}{
		{
			name: "complete metadata",
			metaData: map[string]interface{}{
				"title":       "Test Post",
				"description": "A test description",
				"date":        "2026-02-12",
				"tags":        []interface{}{"go", "testing", "ssg"},
				"pinned":      true,
			},
			wantErr: false,
		},
		{
			name:     "empty metadata",
			metaData: map[string]interface{}{},
			wantErr:  false,
		},
		{
			name: "only title",
			metaData: map[string]interface{}{
				"title": "Just a Title",
			},
			wantErr: false,
		},
		{
			name: "tags unsorted",
			metaData: map[string]interface{}{
				"title": "Post with Tags",
				"tags":  []interface{}{"zebra", "alpha", "beta"},
			},
			wantErr: false,
		},
		{
			name: "pinned false",
			metaData: map[string]interface{}{
				"title":  "Not Pinned",
				"pinned": false,
			},
			wantErr: false,
		},
		{
			name: "with special characters",
			metaData: map[string]interface{}{
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
			if len(hash) != 64 { // SHA256 hex string length
				t.Errorf("GetFrontmatterHash() returned hash of length %d, want 64", len(hash))
			}
		})
	}
}

func TestGetFrontmatterHashDeterministic(t *testing.T) {
	metaData := map[string]interface{}{
		"title":       "Test Post",
		"description": "A test description",
		"date":        "2026-02-12",
		"tags":        []interface{}{"go", "testing"},
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

func TestGetFrontmatterHashTagSorting(t *testing.T) {
	// Same tags in different order should produce same hash
	metaData1 := map[string]interface{}{
		"title": "Post",
		"tags":  []interface{}{"zebra", "alpha", "beta"},
	}

	metaData2 := map[string]interface{}{
		"title": "Post",
		"tags":  []interface{}{"alpha", "beta", "zebra"},
	}

	hash1, _ := GetFrontmatterHash(metaData1)
	hash2, _ := GetFrontmatterHash(metaData2)

	if hash1 != hash2 {
		t.Errorf("Tag order should not affect hash: %s != %s", hash1, hash2)
	}
}

func TestGetGraphHash(t *testing.T) {
	tests := []struct {
		name     string
		posts    []models.PostMetadata
		wantErr  bool
		wantHash bool
	}{
		{
			name: "multiple posts",
			posts: []models.PostMetadata{
				{Title: "Post 1", Link: "/post1", Tags: []string{"go"}},
				{Title: "Post 2", Link: "/post2", Tags: []string{"testing"}},
				{Title: "Post 3", Link: "/post3", Tags: []string{"go", "ssg"}},
			},
			wantErr:  false,
			wantHash: true,
		},
		{
			name:     "empty posts",
			posts:    []models.PostMetadata{},
			wantErr:  false,
			wantHash: true,
		},
		{
			name: "single post",
			posts: []models.PostMetadata{
				{Title: "Only Post", Link: "/only", Tags: []string{}},
			},
			wantErr:  false,
			wantHash: true,
		},
		{
			name: "post with special characters",
			posts: []models.PostMetadata{
				{Title: "Post with <html> & \"quotes\"", Link: "/special", Tags: []string{"unicode: ñ"}},
			},
			wantErr:  false,
			wantHash: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := GetGraphHash(tt.posts)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetGraphHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantHash && hash == "" {
				t.Error("GetGraphHash() returned empty hash")
			}
			if len(hash) != 64 {
				t.Errorf("GetGraphHash() returned hash of length %d, want 64", len(hash))
			}
		})
	}
}

func TestGetGraphHashDeterministic(t *testing.T) {
	posts := []models.PostMetadata{
		{Title: "Post 1", Link: "/post1", Tags: []string{"go", "ssg"}, DateObj: time.Now()},
		{Title: "Post 2", Link: "/post2", Tags: []string{"testing"}, DateObj: time.Now()},
	}

	hash1, err := GetGraphHash(posts)
	if err != nil {
		t.Fatalf("First hash computation failed: %v", err)
	}

	hash2, err := GetGraphHash(posts)
	if err != nil {
		t.Fatalf("Second hash computation failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("GetGraphHash() not deterministic: %s != %s", hash1, hash2)
	}
}

func TestGetGraphHashDifferentInputs(t *testing.T) {
	posts1 := []models.PostMetadata{
		{Title: "Post A", Link: "/post-a", Tags: []string{"go"}},
	}

	posts2 := []models.PostMetadata{
		{Title: "Post B", Link: "/post-b", Tags: []string{"go"}},
	}

	hash1, _ := GetGraphHash(posts1)
	hash2, _ := GetGraphHash(posts2)

	if hash1 == hash2 {
		t.Error("Different posts should produce different hashes")
	}
}
