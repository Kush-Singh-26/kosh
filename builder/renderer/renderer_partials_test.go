package renderer

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/spf13/afero"
)

func TestPartials_LoadedAndRendered(t *testing.T) {
	fs := afero.NewMemMapFs()
	templateDir := "themes/test-theme/templates"
	partialsDir := filepath.Join(templateDir, "partials")
	_ = fs.MkdirAll(partialsDir, 0755)

	// Create a partial
	partialContent := `<div class="partial-box">{{ . }}</div>`
	_ = afero.WriteFile(fs, filepath.Join(partialsDir, "box.html"), []byte(partialContent), 0644)

	// Create a nested partial
	nestedContent := `<span>Nested: {{ template "partials/box.html" . }}</span>`
	_ = afero.WriteFile(fs, filepath.Join(partialsDir, "inner/item.html"), []byte(nestedContent), 0644)

	// Create a layout that uses both
	layoutContent := `{{ define "content" }}
<main>
    {{ template "partials/box.html" "Direct Call" }}
    {{ template "partials/inner/item.html" "Nested Call" }}
</main>
{{ end }}`
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644)

	r := NewWithFs(Options{
		SourceFs:    fs,
		TemplateDir: templateDir,
		Logger:      slog.Default(),
	})

	data := models.PageData{}
	r.PreparePageData(&data)

	buf := new(bytes.Buffer)
	err := r.Layout.Execute(buf, data)
	if err != nil {
		t.Fatalf("Failed to render layout with partials: %v", err)
	}

	output := buf.String()
	expectedDirect := `<div class="partial-box">Direct Call</div>`
	if !strings.Contains(output, expectedDirect) {
		t.Errorf("Output missing direct partial render.\nExpected to contain: %s\nOutput: %s", expectedDirect, output)
	}

	expectedNested := `<span>Nested: <div class="partial-box">Nested Call</div></span>`
	if !strings.Contains(output, expectedNested) {
		t.Errorf("Output missing nested partial render.\nExpected to contain: %s\nOutput: %s", expectedNested, output)
	}
}

func TestPartials_CacheInvalidation(t *testing.T) {
	fs := afero.NewMemMapFs()
	templateDir := "themes/test-theme-cached/templates"
	partialsDir := filepath.Join(templateDir, "partials")
	_ = fs.MkdirAll(partialsDir, 0755)

	_ = afero.WriteFile(fs, filepath.Join(partialsDir, "var.html"), []byte(`v1`), 0644)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(`{{ define "content" }}{{ template "partials/var.html" . }}{{ end }}`), 0644)

	r := NewWithFs(Options{
		SourceFs:    fs,
		TemplateDir: templateDir,
		DevMode:     true, // Ensure small TTL
		Logger:      slog.Default(),
	})

	check := func(expected string) {
		data := models.PageData{}
		r.PreparePageData(&data)
		buf := new(bytes.Buffer)
		err := r.Layout.Execute(buf, data)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but got %q", expected, output)
		}
	}

	check("v1")

	// Ensure TTL passes
	time.Sleep(200 * time.Millisecond)

	// Update partial
	_ = afero.WriteFile(fs, filepath.Join(partialsDir, "var.html"), []byte(`v2`), 0644)

	// Force reload
	r.ReloadTemplates()
	check("v2")
}
