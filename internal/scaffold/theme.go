package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed theme_templates/*
var themeTemplates embed.FS

// Theme scaffolds a new theme directory with boilerplate files.
func Theme(targetPath, themeName string) error {
	// Check if directory already exists
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("theme directory already exists: %s", targetPath)
	}

	// Create directory structure
	dirs := []string{
		targetPath,
		filepath.Join(targetPath, "static", "css"),
		filepath.Join(targetPath, "static", "js"),
		filepath.Join(targetPath, "templates"),
		filepath.Join(targetPath, "templates", "partials"),
		filepath.Join(targetPath, "templates", "shortcodes"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Process and write files
	files := map[string]string{
		"theme_templates/theme.yaml.tmpl":             "theme.yaml",
		"theme_templates/layout.html.tmpl":            "templates/layout.html",
		"theme_templates/index.html.tmpl":             "templates/index.html",
		"theme_templates/404.html.tmpl":               "templates/404.html",
		"theme_templates/layout.css.tmpl":             "static/css/layout.css",
		"theme_templates/post-card.html.tmpl":         "templates/partials/post-card.html",
		"theme_templates/shortcode-youtube.html.tmpl": "templates/shortcodes/youtube.html",
	}

	data := map[string]string{
		"ThemeName": themeName,
	}

	for src, dest := range files {
		content, err := themeTemplates.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read embedded template %s: %w", src, err)
		}

		tmpl, err := template.New(dest).Parse(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", src, err)
		}

		f, err := os.Create(filepath.Join(targetPath, dest))
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", dest, err)
		}

		if err := tmpl.Execute(f, data); err != nil {
			_ = f.Close()
			return fmt.Errorf("failed to execute template %s: %w", dest, err)
		}
		_ = f.Close()
	}

	return nil
}
