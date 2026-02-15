package run

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
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
			path:      "content/post.md",
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
			b := &Builder{
				cfg: &config.Config{
					StaticDir: tt.staticDir,
				},
			}
			got := b.isAssetPath(tt.path)
			if got != tt.want {
				t.Errorf("isAssetPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
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
			name:         "layout.html changes affect all",
			templatePath: "themes/test-theme/templates/layout.html",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "static file changes affect all",
			templatePath: "themes/test-theme/static/css/style.css",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "kosh.yaml changes affect all",
			templatePath: "kosh.yaml",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "pwa.go changes return empty",
			templatePath: "builder/generators/pwa.go",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Builder{
				cfg: &config.Config{
					TemplateDir: tt.templateDir,
					StaticDir:   tt.staticDir,
				},
			}
			got := b.invalidateForTemplate(tt.templatePath)
			if (got == nil) != tt.wantNil {
				t.Errorf("invalidateForTemplate(%q) returned nil=%v, want nil=%v", tt.templatePath, got == nil, tt.wantNil)
			}
		})
	}
}
