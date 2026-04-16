package watch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/config"
)

func TestCoordinator_ClassifyChange(t *testing.T) {
	wd, _ := os.Getwd()
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: filepath.Join(wd, "content"),
			StaticDir:  filepath.Join(wd, "themes/blog/static"),
		},
		SiteRoot: wd,
	}

	c := New(CoordinatorDependencies{
		Cfg: cfg,
	})

	tests := []struct {
		name     string
		path     string
		op       fsnotify.Op
		expected ChangeType
	}{
		{
			name:     "Markdown Content",
			path:     filepath.Join(wd, "content/blog/Content.md"),
			op:       fsnotify.Write,
			expected: ChangeTypeContent,
		},
		{
			name:     "Asset File",
			path:     filepath.Join(wd, "themes/blog/static/css/style.css"),
			op:       fsnotify.Write,
			expected: ChangeTypeAsset,
		},
		{
			name:     "Site Static File",
			path:     filepath.Join(wd, "static/js/app.js"),
			op:       fsnotify.Write,
			expected: ChangeTypeAsset,
		},
		{
			name:     "Delete Operation",
			path:     filepath.Join(wd, "content/blog/Content.md"),
			op:       fsnotify.Remove,
			expected: ChangeTypeDelete,
		},
		{
			name:     "Other File",
			path:     filepath.Join(wd, "kosh.yaml"),
			op:       fsnotify.Write,
			expected: ChangeTypeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ClassifyChange(tt.path, tt.op)
			if got.Type != tt.expected {
				t.Errorf("ClassifyChange(%s) type = %v, want %v", tt.path, got.Type, tt.expected)
			}
		})
	}
}

func TestIsSearchSourcePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"cmd/search/main.go", true},
		{"builder/search/core.go", true},
		{"builder/models/models.go", true},
		{"content/blog/Content.md", false},
		{"static/js/app.js", false},
	}

	for _, tt := range tests {
		if got := IsSearchSourcePath(tt.path); got != tt.expected {
			t.Errorf("IsSearchSourcePath(%s) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestCoordinator_IsContentPath(t *testing.T) {
	wd, _ := os.Getwd()
	contentDir := filepath.Join(wd, "content")
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: contentDir,
		},
	}
	c := New(CoordinatorDependencies{Cfg: cfg})

	path := filepath.Join(contentDir, "Content.md")
	if !c.IsContentPath(path) {
		t.Errorf("expected %s to be content path", path)
	}

	path = filepath.Join(wd, "other/Content.md")
	if c.IsContentPath(path) {
		t.Errorf("expected %s NOT to be content path", path)
	}
}
