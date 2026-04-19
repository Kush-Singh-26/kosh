package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestIsAssetPath(t *testing.T) {
	staticDir := "themes/test-theme/static"
	tests := []struct {
		name      string
		path      string
		staticDir string
		want      bool
	}{
		{
			name:      "css in theme static",
			path:      "themes/test-theme/static/css/style.css",
			staticDir: staticDir,
			want:      true,
		},
		{
			name:      "js in site static",
			path:      "static/js/main.js",
			staticDir: staticDir,
			want:      true,
		},
		{
			name:      "markdown file",
			path:      "content/Content.md",
			staticDir: staticDir,
			want:      false,
		},
		{
			name:      "config file",
			path:      "kosh.yaml",
			staticDir: staticDir,
			want:      false,
		},
		{
			name:      "nested css",
			path:      "themes/test-theme/static/css/nested/style.css",
			staticDir: staticDir,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := InitLogger()
			cfg := &config.Config{
				PathConfig: config.PathConfig{
					StaticDir: tt.staticDir,
				},
			}
			b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
			got := b.Watch.IsAssetPath(tt.path)
			if got != tt.want {
				t.Errorf("isAssetPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeWatchPath_ProjectRelativeAbsolutePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	logger := InitLogger()
	cfg := &config.Config{}
	b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
	abs := filepath.Join(wd, "themes", "test-theme", "static", "css", "style.css")
	got := b.Watch.NormalizeWatchPath(abs)
	expected := fspkg.NormalizePath("themes/test-theme/static/css/style.css")
	if got != expected {
		t.Fatalf("got %q, want %q", got, expected)
	}
}

func TestIsContentPathWithAbsoluteConfiguredContentDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	contentDir := filepath.Join(wd, "content")
	logger := InitLogger()
	cfg := &config.Config{PathConfig: config.PathConfig{ContentDir: contentDir}}
	b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Logger: logger}))
	path := filepath.Join(contentDir, "posts", "hello.md")
	if !b.Watch.IsContentPath(path) {
		t.Fatalf("expected absolute markdown path to match absolute content dir")
	}
}

func TestInvalidateForTemplate(t *testing.T) {
	templateDir := "themes/test-theme/templates"
	staticDir := "themes/test-theme/static"
	tests := []struct {
		name         string
		templatePath string
		templateDir  string
		staticDir    string
		wantNil      bool
	}{
		{
			name:         "layout.html_changes_affect_all",
			templatePath: "themes/test-theme/templates/layout.html",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "static_file_changes_affect_all",
			templatePath: "themes/test-theme/static/css/style.css",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "kosh.yaml_changes_affect_all",
			templatePath: "kosh.yaml",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "pwa.go_changes_return_empty",
			templatePath: "builder/generators/pwa.go",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := InitLogger()
			cfg := &config.Config{
				PathConfig: config.PathConfig{
					TemplateDir: tt.templateDir,
					StaticDir:   tt.staticDir,
				},
			}
			b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
			got := b.Watch.InvalidateForTemplate(tt.templatePath)
			if (got == nil) != tt.wantNil {
				t.Errorf("invalidateForTemplate(%q) returned nil=%v, want nil=%v", tt.templatePath, got == nil, tt.wantNil)
			}
		})
	}
}

func TestModTimeQuickBail(t *testing.T) {
	cachedMeta := &models.ContentMeta{
		ModTime:  1000,
		BodyHash: "hash123",
	}

	info := afero.NewMemMapFs()
	_ = afero.WriteFile(info, "Content.md", []byte("content"), 0644)
	stat, _ := info.Stat("Content.md")

	shouldForce := false
	exists := true

	fastBail := !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash != "" && stat != nil && cachedMeta.ModTime == stat.ModTime().Unix()

	if fastBail {
		t.Error("fastBail should be false when ModTime mismatches")
	}
}
