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
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "string value",
			m:        map[string]any{"title": "Hello"},
			key:      "title",
			expected: "Hello",
		},
		{
			name:     "int value",
			m:        map[string]any{"count": 42},
			key:      "count",
			expected: "42",
		},
		{
			name:     "bool value",
			m:        map[string]any{"active": true},
			key:      "active",
			expected: "true",
		},
		{
			name:     "missing key",
			m:        map[string]any{"other": "value"},
			key:      "missing",
			expected: "",
		},
		{
			name:     "empty map",
			m:        map[string]any{},
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
			m:        map[string]any{"tags": []string{"go", "ssg"}},
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
		m        map[string]any
		key      string
		expected []string
	}{
		{
			name:     "valid slice",
			m:        map[string]any{"tags": []any{"go", "ssg", "web"}},
			key:      "tags",
			expected: []string{"go", "ssg", "web"},
		},
		{
			name:     "mixed types in slice",
			m:        map[string]any{"items": []any{"string", 42, true}},
			key:      "items",
			expected: []string{"string", "42", "true"},
		},
		{
			name:     "missing key",
			m:        map[string]any{"other": "value"},
			key:      "tags",
			expected: nil,
		},
		{
			name:     "empty map",
			m:        map[string]any{},
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
			m:        map[string]any{"tags": "go,ssg"},
			key:      "tags",
			expected: nil,
		},
		{
			name:     "empty slice",
			m:        map[string]any{"tags": []any{}},
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
		m        map[string]any
		key      string
		expected bool
	}{
		{
			name:     "true value",
			m:        map[string]any{"pinned": true},
			key:      "pinned",
			expected: true,
		},
		{
			name:     "false value",
			m:        map[string]any{"pinned": false},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "missing key",
			m:        map[string]any{"other": "value"},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "empty map",
			m:        map[string]any{},
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
			m:        map[string]any{"pinned": "true"},
			key:      "pinned",
			expected: false,
		},
		{
			name:     "wrong type (int)",
			m:        map[string]any{"pinned": 1},
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

func TestCountWords(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Hello world", 2},
		{"  Multiple   spaces  ", 2},
		{"Line\nbreaks\rand\ttabs", 4},
		{"", 0},
		{"OneWord", 1},
		{"!@#$% ^&*()", 2}, // basic non-space counting
		{"Unicode 🚀 rocks", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CountWords([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("CountWords(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
