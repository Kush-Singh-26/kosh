package renderer

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
)

func TestRenderer_ReloadTemplates(t *testing.T) {
	fs := afero.NewMemMapFs()
	sink := mocks.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create dummy template files
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ .Title }}`), 0644)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "index.html"), []byte(`Index: {{ .Title }}`), 0644)

	r := NewWithFs(fs, false, sink, templateDir, false, logger)

	if r.Layout == nil {
		t.Fatal("Layout template should not be nil")
	}
	if r.Index == nil {
		t.Fatal("Index template should not be nil")
	}

	// Verify template execution
	buf := new(bytes.Buffer)
	_ = r.Layout.Execute(buf, map[string]string{"Title": "My Page"})
	if buf.String() != "My Page" {
		t.Errorf("Expected My Page, got %s", buf.String())
	}
}

func TestRenderer_ReloadTemplates_Missing(t *testing.T) {
	fs := afero.NewMemMapFs()
	sink := mocks.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create only layout.html (minimal required)
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`Layout`), 0644)

	r := NewWithFs(fs, false, sink, templateDir, false, logger)

	if r.Layout == nil {
		t.Fatal("Layout template should not be nil")
	}
	if r.Index != nil {
		t.Error("Expected nil index template when file is missing")
	}
}

func TestRenderer_FuncMap_Relativize(t *testing.T) {
	// Helper functions are inside FuncMap in ReloadTemplates
	// I'll test them by executing a small template
	fs := afero.NewMemMapFs()
	sink := mocks.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ relativize .BaseURL .RelativePrefix .Link }}`), 0644)

	r := NewWithFs(fs, false, sink, templateDir, false, logger)

	tests := []struct {
		baseURL  string
		prefix   string
		link     string
		expected string
	}{
		{"", "../", "/posts/test.html", "../posts/test.html"},
		{"https://example.com", "../", "/posts/test.html", "https://example.com/posts/test.html"},
		{"", "", "/posts/test.html", "posts/test.html"},
		{"", "", "/", "index.html"},
		{"", "../", "/", "../index.html"},
		{"", "", "https://external.com", "https://external.com"},
	}

	for _, tt := range tests {
		buf := new(bytes.Buffer)
		data := struct {
			BaseURL        string
			RelativePrefix string
			Link           string
		}{tt.baseURL, tt.prefix, tt.link}

		err := r.Layout.Execute(buf, data)
		if err != nil {
			t.Errorf("Template execution failed for %v: %v", tt, err)
			continue
		}

		if buf.String() != tt.expected {
			t.Errorf("relativize(%q, %q, %q) = %q, want %q", tt.baseURL, tt.prefix, tt.link, buf.String(), tt.expected)
		}
	}
}

func TestRenderer_FuncMap_Slugify(t *testing.T) {
	fs := afero.NewMemMapFs()
	sink := mocks.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ slugify .Title }}`), 0644)

	r := NewWithFs(fs, false, sink, templateDir, false, logger)

	buf := new(bytes.Buffer)
	_ = r.Layout.Execute(buf, map[string]string{"Title": "Hello World!"})
	if buf.String() != "hello-world" {
		t.Errorf("Expected hello-world, got %s", buf.String())
	}
}
