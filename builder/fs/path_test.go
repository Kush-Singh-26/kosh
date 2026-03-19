package fs

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/afero"
)

func TestNormalizePath_Basic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "."},
		{".", "."},
		{"foo", "foo"},
		{"foo/bar", "foo/bar"},
		{"foo/bar/", "foo/bar/"}, // trailing slash preserved after clean
		{"foo//bar", "foo/bar"},
		{"foo/./bar", "foo/bar"},
		{"foo/../bar", "bar"},
		{"/foo/bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePath_Backslashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"foo\\bar", "foo/bar"},
		{"foo\\bar\\baz", "foo/bar/baz"},
		{"C:\\Users\\test", "C:/Users/test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePath_WindowsDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"c:\\foo", "C:/foo"},
		{"d:\\bar\\baz", "D:/bar/baz"},
		{"C:\\Users", "C:/Users"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePath_Unicode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"café", "café"},
		{"日本語", "日本語"},
		{"test/日本語", "test/日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAbsNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		checkAbs bool // if true, just check result is absolute
	}{
		{".", false, true},
		{"foo", false, true},
		{"foo/bar", false, true},
		{"/absolute/path", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := AbsNormalizePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AbsNormalizePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tt.checkAbs && !filepath.IsAbs(result) {
					t.Errorf("AbsNormalizePath(%q) = %q, want absolute path", tt.input, result)
				}
			}
		})
	}
}

func TestNormalizeURLPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "."},
		{"/", "/"},
		{"/foo", "/foo"},
		{"/foo/bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
		{"\\foo\\bar", "/foo/bar"},
		{"/foo//bar", "/foo/bar"},
		{"/foo/./bar", "/foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeURLPath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeURLPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeWatchPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wd       string
		expected string
	}{
		{"no wd", "foo/bar", "", "foo/bar"},
		{"absolute path with wd - escapes", "/home/user/project/foo", "/home/user/project", "/home/user/project/foo"}, // doesn't make relative if it escapes
		{"relative path", "foo/bar", "/home/user", "foo/bar"},
		{"empty path", "", "", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeWatchPath(tt.path, tt.wd)
			if result != tt.expected {
				t.Errorf("NormalizeWatchPath(%q, %q) = %q, want %q", tt.path, tt.wd, result, tt.expected)
			}
		})
	}
}

func TestSafeRel(t *testing.T) {
	tests := []struct {
		base    string
		target  string
		want    string
		wantErr bool
	}{
		{"/home/user", "/home/user/foo", "foo", false},
		{"/home/user", "/home/user/foo/bar", "foo/bar", false},
		{"/home/user", "/home/other", "", true}, // path traversal
		{"/home/user", "/etc/passwd", "", true}, // path traversal
		{"", "", ".", false},                    // Rel("", "") = "."
	}

	for _, tt := range tests {
		t.Run(tt.base+"_"+tt.target, func(t *testing.T) {
			result, err := SafeRel(tt.base, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeRel(%q, %q) error = %v, wantErr %v", tt.base, tt.target, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.want {
				t.Errorf("SafeRel(%q, %q) = %q, want %q", tt.base, tt.target, result, tt.want)
			}
		})
	}
}

func TestWriteFileVFS(t *testing.T) {
	fs := afero.NewMemMapFs()

	err := WriteFileVFS(fs, "foo/bar.txt", []byte("test content"))
	if err != nil {
		t.Fatalf("WriteFileVFS failed: %v", err)
	}

	// Verify file exists
	exists, err := afero.Exists(fs, "foo/bar.txt")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if !exists {
		t.Error("WriteFileVFS did not create file")
	}

	// Verify content
	content, err := afero.ReadFile(fs, "foo/bar.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("WriteFileVFS content = %q, want %q", string(content), "test content")
	}
}

func TestGetRelativePrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{".", ""},
		{"index.html", ""},
		{"foo.html", ""},               // no slashes = depth 0
		{"foo/bar.html", "../"},        // 1 slash = depth 1
		{"foo/bar/baz.html", "../../"}, // 2 slashes = depth 2
		{"a/b/c/d.html", "../../../"},  // 3 slashes = depth 3
		{"/foo/bar.html", "../"},       // leading slash ignored, 1 slash = depth 1
		{"/a/b/c/d.html", "../../../"}, // leading slash ignored, 3 slashes = depth 3
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GetRelativePrefix(tt.input)
			if result != tt.expected {
				t.Errorf("GetRelativePrefix(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetRealPath(t *testing.T) {
	t.Run("nil fs", func(t *testing.T) {
		path, ok := GetRealPath(nil, "/test")
		if ok {
			t.Error("GetRealPath(nil) should return false")
		}
		if path != "" {
			t.Errorf("GetRealPath(nil) path = %q, want empty", path)
		}
	})

	t.Run("OsFs", func(t *testing.T) {
		fs := afero.NewOsFs()
		path, ok := GetRealPath(fs, "/test")
		if !ok {
			t.Error("GetRealPath(OsFs) should return true")
		}
		if path != "/test" {
			t.Errorf("GetRealPath(OsFs) path = %q, want /test", path)
		}
	})

	t.Run("MemMapFs", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		_, ok := GetRealPath(fs, "/test")
		if ok {
			t.Error("GetRealPath(MemMapFs) should return false")
		}
	})
}

func TestIsPathInOrSame(t *testing.T) {
	tests := []struct {
		path     string
		target   string
		expected bool
	}{
		{"/home/user/foo", "/home/user", true},
		{"/home/user/foo/bar", "/home/user", true},
		{"/home/user", "/home/user", true},
		{"/home/other", "/home/user", false},
		{"foo/bar", "foo", true},
		{"foo", "foo", true},
		{"bar", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.target, func(t *testing.T) {
			result := IsPathInOrSame(tt.path, tt.target)
			if result != tt.expected {
				t.Errorf("IsPathInOrSame(%q, %q) = %v, want %v", tt.path, tt.target, result, tt.expected)
			}
		})
	}
}

func TestRepoRoot(t *testing.T) {
	root := RepoRoot()
	if root == "" {
		t.Error("RepoRoot() returned empty string")
	}
	if !filepath.IsAbs(root) {
		t.Errorf("RepoRoot() = %q, want absolute path", root)
	}
}

func TestRepoPath(t *testing.T) {
	path := RepoPath("foo", "bar")
	if path == "" {
		t.Error("RepoPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("RepoPath() = %q, want absolute path", path)
	}
}

func TestHasNonASCII(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ascii", false},
		{"café", true},
		{"日本語", true},
		{"", false},
		{"test123", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := hasNonASCII(tt.input)
			if result != tt.expected {
				t.Errorf("hasNonASCII(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
