package renderer

import (
	"bytes"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/spf13/afero"
)

func TestRenderer_ReloadTemplates(t *testing.T) {
	fs := afero.NewMemMapFs()
	sink := testutil.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create dummy template files in slot format
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ define "content" }}{{ .Title }}{{ end }}`), 0644)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "index.html"), []byte(`{{ define "content" }}Index: {{ .Title }}{{ end }}`), 0644)

	r := NewWithFs(Options{SourceFs: fs, Compress: false, Sink: sink, TemplateDir: templateDir, DevMode: false, Logger: logger})

	if r.Layout == nil {
		t.Fatal("Layout template should not be nil")
	}
	if r.Index == nil {
		t.Fatal("Index template should not be nil")
	}

	// Verify template execution - now it contains the base chrome
	buf := new(bytes.Buffer)
	_ = r.Layout.Execute(buf, map[string]any{"Title": "My Page", "TabTitle": "Tab", "Description": "Desc"})
	if !strings.Contains(buf.String(), "My Page") {
		t.Errorf("Expected output to contain 'My Page', got %s", buf.String())
	}
}

func TestRenderer_ReloadTemplates_Missing(t *testing.T) {
	fs := afero.NewMemMapFs()
	sink := testutil.NewMemSink()
	templateDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create only layout.html (minimal required)
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ define "content" }}Layout{{ end }}`), 0644)

	r := NewWithFs(Options{SourceFs: fs, Compress: false, Sink: sink, TemplateDir: templateDir, DevMode: false, Logger: logger})

	if r.Layout == nil {
		t.Fatal("Layout template should not be nil")
	}
	if r.Index != nil {
		t.Error("Expected nil index template when file is missing")
	}
}

func TestRenderer_FuncMap_Relativize(t *testing.T) {
	// Test the function in isolation to avoid base.html field requirements
	funcMap := templateFuncMap()
	tmpl := template.Must(template.New("test").Funcs(funcMap).Parse(`{{ relativize .BaseURL .RelativePrefix .Link }}`))

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

		err := tmpl.Execute(buf, data)
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
	// Test the function in isolation
	funcMap := templateFuncMap()
	tmpl := template.Must(template.New("test").Funcs(funcMap).Parse(`{{ slugify .Title }}`))

	buf := new(bytes.Buffer)
	_ = tmpl.Execute(buf, map[string]string{"Title": "Hello World!"})
	if buf.String() != "hello-world" {
		t.Errorf("Expected hello-world, got %s", buf.String())
	}
}

