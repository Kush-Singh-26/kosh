package utils

import (
	"path/filepath"
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
		{"./posts/hello.html", "posts/hello.html"},
		{"cafe\u0301", "caf\u00e9"},
	}

	for _, tt := range tests {
		got := NormalizePath(tt.input)
		expected := tt.expected
		if got != expected {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, expected)
		}
	}

	if runtime.GOOS == "windows" {
		got := NormalizePath("c:/path/file.txt")
		if got != "C:/path/file.txt" {
			t.Errorf("NormalizePath drive case = %q, want %q", got, "C:/path/file.txt")
		}
	}
}

func TestAbsNormalizePath(t *testing.T) {
	abs, err := filepath.Abs("content")
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	got, err := AbsNormalizePath("content")
	if err != nil {
		t.Fatalf("AbsNormalizePath error: %v", err)
	}
	want := NormalizePath(abs)
	if got != want {
		t.Errorf("AbsNormalizePath(%q) = %q, want %q", "content", got, want)
	}
}

func TestNormalizeURLPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a\\b\\c", "a/b/c"},
		{"/a/../b", "/b"},
		{"a//b", "a/b"},
		{"", "."},
	}
	for _, tt := range tests {
		got := NormalizeURLPath(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeURLPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeWatchPath(t *testing.T) {
	wd := t.TempDir()
	abs := filepath.Join(wd, "themes", "test-theme", "static", "css", "style.css")
	got := NormalizeWatchPath(abs, wd)
	if got != "themes/test-theme/static/css/style.css" {
		t.Fatalf("NormalizeWatchPath returned %q", got)
	}

	other := t.TempDir()
	absOther := filepath.Join(other, "file.txt")
	gotOther := NormalizeWatchPath(absOther, wd)
	wantOther := NormalizePath(absOther)
	if gotOther != wantOther {
		t.Fatalf("NormalizeWatchPath outside wd = %q, want %q", gotOther, wantOther)
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
