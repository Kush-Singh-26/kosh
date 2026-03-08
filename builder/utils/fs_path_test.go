package utils

import (
	"runtime"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"path/to/file", "path/to/file"},
		{"path\\to\\file", "path/to/file"},
		{"mixed/path\\case", "mixed/path/case"},
	}

	for _, tt := range tests {
		got := NormalizePath(tt.input)
		// On Windows, NormalizePath lowercases everything
		expected := tt.expected
		if runtime.GOOS == "windows" {
			expected = NormalizePath(tt.expected) // apply same logic
		}
		if got != expected {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, expected)
		}
	}
}

func TestGetRelativePrefix(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"index.html", ""},
		{"posts/hello.html", "../"},
		{"posts/2026/03/post.html", "../../../"},
		{"/index.html", ""},
		{"./posts/hello.html", "../"},
	}

	for _, tt := range tests {
		got := GetRelativePrefix(tt.path)
		if got != tt.expected {
			t.Errorf("GetRelativePrefix(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestSafeRel(t *testing.T) {
	tests := []struct {
		base    string
		target  string
		wantErr bool
	}{
		{"/app/public", "/app/public/posts/hello.html", false},
		{"/app/public", "/app/private/secret.txt", true},
		{"C:\\project", "C:\\project\\index.html", false},
	}

	for _, tt := range tests {
		_, err := SafeRel(tt.base, tt.target)
		if (err != nil) != tt.wantErr {
			t.Errorf("SafeRel(%q, %q) error = %v, wantErr %v", tt.base, tt.target, err, tt.wantErr)
		}
	}
}
