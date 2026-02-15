package utils

import (
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestSortPosts(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		posts    []models.PostMetadata
		expected []string // Expected order of titles
	}{
		{
			name: "sort by weight descending",
			posts: []models.PostMetadata{
				{Title: "Low Weight", Weight: 1, DateObj: now},
				{Title: "High Weight", Weight: 10, DateObj: now},
				{Title: "Medium Weight", Weight: 5, DateObj: now},
			},
			expected: []string{"High Weight", "Medium Weight", "Low Weight"},
		},
		{
			name: "same weight sort by date descending",
			posts: []models.PostMetadata{
				{Title: "Old", Weight: 5, DateObj: now.Add(-24 * time.Hour)},
				{Title: "New", Weight: 5, DateObj: now},
				{Title: "Medium", Weight: 5, DateObj: now.Add(-12 * time.Hour)},
			},
			expected: []string{"New", "Medium", "Old"},
		},
		{
			name: "same weight and date sort by title descending",
			posts: []models.PostMetadata{
				{Title: "Apple", Weight: 5, DateObj: now},
				{Title: "Zebra", Weight: 5, DateObj: now},
				{Title: "Banana", Weight: 5, DateObj: now},
			},
			expected: []string{"Zebra", "Banana", "Apple"},
		},
		{
			name: "mixed weight and date",
			posts: []models.PostMetadata{
				{Title: "Heavy Old", Weight: 10, DateObj: now.Add(-24 * time.Hour)},
				{Title: "Light New", Weight: 1, DateObj: now},
				{Title: "Heavy New", Weight: 10, DateObj: now},
			},
			expected: []string{"Heavy New", "Heavy Old", "Light New"},
		},
		{
			name:     "empty slice",
			posts:    []models.PostMetadata{},
			expected: []string{},
		},
		{
			name: "single post",
			posts: []models.PostMetadata{
				{Title: "Only", Weight: 5, DateObj: now},
			},
			expected: []string{"Only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortPosts(tt.posts)

			if len(tt.posts) != len(tt.expected) {
				t.Fatalf("got %d posts, want %d", len(tt.posts), len(tt.expected))
			}

			for i, post := range tt.posts {
				if post.Title != tt.expected[i] {
					t.Errorf("position %d: got %q, want %q", i, post.Title, tt.expected[i])
				}
			}
		})
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected string
	}{
		{
			name:     "string value",
			m:        map[string]interface{}{"title": "Hello"},
			key:      "title",
			expected: "Hello",
		},
		{
			name:     "int value",
			m:        map[string]interface{}{"count": 42},
			key:      "count",
			expected: "42",
		},
		{
			name:     "bool value",
			m:        map[string]interface{}{"active": true},
			key:      "active",
			expected: "true",
		},
		{
			name:     "missing key",
			m:        map[string]interface{}{"other": "value"},
			key:      "missing",
			expected: "",
		},
		{
			name:     "empty map",
			m:        map[string]interface{}{},
			key:      "anything",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "key",
			expected: "",
		},
		{
			name:     "slice value",
			m:        map[string]interface{}{"tags": []string{"go", "ssg"}},
			key:      "tags",
			expected: "[go ssg]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetString(tt.m, tt.key)
			if result != tt.expected {
				t.Errorf("GetString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetSlice(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected []string
	}{
		{
			name:     "valid slice",
			m:        map[string]interface{}{"tags": []interface{}{"go", "ssg", "web"}},
			key:      "tags",
			expected: []string{"go", "ssg", "web"},
		},
		{
			name:     "mixed types in slice",
			m:        map[string]interface{}{"items": []interface{}{"string", 42, true}},
			key:      "items",
			expected: []string{"string", "42", "true"},
		},
		{
			name:     "missing key",
			m:        map[string]interface{}{"other": "value"},
			key:      "tags",
			expected: nil,
		},
		{
			name:     "empty map",
			m:        map[string]interface{}{},
			key:      "tags",
			expected: nil,
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "tags",
			expected: nil,
		},
		{
			name:     "wrong type (string instead of slice)",
			m:        map[string]interface{}{"tags": "go,ssg"},
			key:      "tags",
			expected: nil,
		},
		{
			name:     "empty slice",
			m:        map[string]interface{}{"tags": []interface{}{}},
			key:      "tags",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSlice(tt.m, tt.key)

			if len(result) != len(tt.expected) {
				t.Errorf("GetSlice() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("GetSlice()[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestGetBool(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected bool
	}{
		{
			name:     "true value",
			m:        map[string]interface{}{"pinned": true},
			key:      "pinned",
			expected: true,
		},
		{
			name:     "false value",
			m:        map[string]interface{}{"pinned": false},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "missing key",
			m:        map[string]interface{}{"other": "value"},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "empty map",
			m:        map[string]interface{}{},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "pinned",
			expected: false,
		},
		{
			name:     "wrong type (string)",
			m:        map[string]interface{}{"pinned": "true"},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "wrong type (int)",
			m:        map[string]interface{}{"pinned": 1},
			key:      "pinned",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBool(tt.m, tt.key)
			if result != tt.expected {
				t.Errorf("GetBool() = %v, want %v", result, tt.expected)
			}
		})
	}
}
