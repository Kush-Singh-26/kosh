package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// changeToTempDir changes to a temp directory and returns a cleanup function
func changeToTempDir(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	return func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Change to a temp directory to avoid loading actual kosh.yaml
	cleanup := changeToTempDir(t)
	defer cleanup()

	cfg := Load([]string{})

	// Check default values
	if cfg.Title != "Kosh Blog" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Kosh Blog")
	}

	if cfg.PostsPerPage != 10 {
		t.Errorf("PostsPerPage = %d, want 10", cfg.PostsPerPage)
	}

	// Default ImageWorkers is 8
	if cfg.ImageWorkers != 8 {
		t.Errorf("ImageWorkers = %d, want 8", cfg.ImageWorkers)
	}

	if cfg.Theme != "blog" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "blog")
	}

	if cfg.ContentDir == "" {
		t.Error("ContentDir should not be empty")
	}

	if cfg.OutputDir == "" {
		t.Error("OutputDir should not be empty")
	}

	if cfg.CacheDir == "" {
		t.Error("CacheDir should not be empty")
	}

	// Check default features
	if !cfg.Features.Generators.Sitemap {
		t.Error("Sitemap generator should be enabled by default")
	}

	if !cfg.Features.Generators.RSS {
		t.Error("RSS generator should be enabled by default")
	}

	if !cfg.Features.Generators.Graph.Enabled {
		t.Error("Graph generator should be enabled by default")
	}
	if !cfg.Features.Generators.Graph.ShowTags {
		t.Error("Graph showTags should be enabled by default")
	}

	if !cfg.Features.Generators.Search {
		t.Error("Search generator should be enabled by default")
	}
}

func TestLoad_FromYAML(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	// Create a test kosh.yaml
	yamlContent := `
title: "Test Site"
description: "A test site"
baseURL: "https://test.example.com"
postsPerPage: 20
theme: "docs"
author:
  name: "Test Author"
  url: "https://author.example.com"
features:
  generators:
    sitemap: false
    rss: false
`
	if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test kosh.yaml: %v", err)
	}

	cfg := Load([]string{})

	if cfg.Title != "Test Site" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Test Site")
	}

	if cfg.Description != "A test site" {
		t.Errorf("Description = %q, want %q", cfg.Description, "A test site")
	}

	if cfg.BaseURL != "https://test.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://test.example.com")
	}

	if cfg.PostsPerPage != 20 {
		t.Errorf("PostsPerPage = %d, want 20", cfg.PostsPerPage)
	}

	if cfg.Theme != "docs" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "docs")
	}

	if cfg.Author.Name != "Test Author" {
		t.Errorf("Author.Name = %q, want %q", cfg.Author.Name, "Test Author")
	}

	if cfg.Features.Generators.Sitemap {
		t.Error("Sitemap should be disabled")
	}

	if cfg.Features.Generators.RSS {
		t.Error("RSS should be disabled")
	}
}

func TestLoad_FallbackConfigYaml(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	// Create a test config.yaml (fallback)
	yamlContent := `
title: "Fallback Site"
baseURL: "https://fallback.example.com"
`
	if err := os.WriteFile("config.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test config.yaml: %v", err)
	}

	cfg := Load([]string{})

	if cfg.Title != "Fallback Site" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Fallback Site")
	}

	if cfg.BaseURL != "https://fallback.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://fallback.example.com")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	// Create an invalid YAML file
	if err := os.WriteFile("kosh.yaml", []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatalf("Failed to create test kosh.yaml: %v", err)
	}

	// Should not panic and should use defaults
	cfg := Load([]string{})

	if cfg.Title != "Kosh Blog" {
		t.Errorf("Title = %q, want default %q", cfg.Title, "Kosh Blog")
	}
}

func TestLoad_CLIOverrides(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	// Create a test kosh.yaml
	yamlContent := `
title: "Test Site"
baseURL: "https://test.example.com"
`
	if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test kosh.yaml: %v", err)
	}

	// Override with CLI flags
	args := []string{"-baseurl", "https://override.example.com", "-drafts"}
	cfg := Load(args)

	if cfg.BaseURL != "https://override.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://override.example.com")
	}

	if !cfg.IncludeDrafts {
		t.Error("IncludeDrafts should be true")
	}
}

func TestLoad_ThemeOverride(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	// Create a test kosh.yaml
	yamlContent := `
theme: "blog"
`
	if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test kosh.yaml: %v", err)
	}

	// Override theme with CLI flag
	args := []string{"-theme", "docs"}
	cfg := Load(args)

	if cfg.Theme != "docs" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "docs")
	}

	// Verify template and static dirs were updated
	expectedTemplateDir := filepath.Join(cfg.ThemeDir, "docs", "templates")
	if cfg.TemplateDir != expectedTemplateDir {
		t.Errorf("TemplateDir = %q, want %q", cfg.TemplateDir, expectedTemplateDir)
	}
}

func TestLoad_AbsolutePaths(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	oldArgs := os.Args
	os.Args = []string{"kosh"}
	defer func() { os.Args = oldArgs }()

	cfg := Load([]string{})

	// All paths should be absolute
	if !filepath.IsAbs(cfg.ContentDir) {
		t.Errorf("ContentDir = %q should be absolute", cfg.ContentDir)
	}

	if !filepath.IsAbs(cfg.OutputDir) {
		t.Errorf("OutputDir = %q should be absolute", cfg.OutputDir)
	}

	if !filepath.IsAbs(cfg.CacheDir) {
		t.Errorf("CacheDir = %q should be absolute", cfg.CacheDir)
	}

	if !filepath.IsAbs(cfg.ThemeDir) {
		t.Errorf("ThemeDir = %q should be absolute", cfg.ThemeDir)
	}

	if !filepath.IsAbs(cfg.TemplateDir) {
		t.Errorf("TemplateDir = %q should be absolute", cfg.TemplateDir)
	}

	if !filepath.IsAbs(cfg.StaticDir) {
		t.Errorf("StaticDir = %q should be absolute", cfg.StaticDir)
	}
}

func TestLoad_ImageWorkersValidation(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		expected  int
		vipsConcy int
	}{
		{"zero_defaults_to_8", 0, 8, 0},
		{"negative_defaults_to_8", -5, 8, 0},
		{"valid_value", 16, 16, 0},
		{"maximum_cap", 100, 32, 0},
		{"at_maximum", 32, 32, 0},
		{"vips_with_valid_workers", 8, 8, 4},
		{"vips_with_high_workers", 16, 16, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := changeToTempDir(t)
			defer cleanup()

			yamlContent := ""
			if tt.workers != 0 || tt.vipsConcy != 0 {
				yamlContent = fmt.Sprintf("imageWorkers: %d\nvipsConcurrency: %d", tt.workers, tt.vipsConcy)
			}
			if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to create test kosh.yaml: %v", err)
			}

			cfg := Load([]string{})

			// The only validation is: 0 -> 8, > 32 -> 32
			expected := tt.expected
			if tt.workers <= 0 {
				expected = 8
			} else if tt.workers > 32 {
				expected = 32
			}

			if cfg.ImageWorkers != expected {
				t.Errorf("ImageWorkers = %d, want %d", cfg.ImageWorkers, expected)
			}
		})
	}
}

func TestSetDevMode(t *testing.T) {
	cfg := &Config{}

	SetDevMode(cfg, true)
	if !cfg.IsDev {
		t.Error("IsDev should be true")
	}

	SetDevMode(cfg, false)
	if cfg.IsDev {
		t.Error("IsDev should be false")
	}
}

func TestLoad_CLIOverrides_Afero(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create a test kosh.yaml in VFS
	yamlContent := `
title: "VFS Site"
baseURL: "https://vfs.example.com"
`
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(yamlContent), 0644)

	// Override with CLI flags
	args := []string{"-baseurl", "https://override.vfs.com", "-drafts", "-theme", "my-theme"}
	cfg := LoadFs(fs, args)

	if cfg.BaseURL != "https://override.vfs.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://override.vfs.com")
	}

	if !cfg.IncludeDrafts {
		t.Error("IncludeDrafts should be true")
	}

	if cfg.Theme != "my-theme" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "my-theme")
	}
}

func TestConfig_SocialCardsDefaults(t *testing.T) {
	cleanup := changeToTempDir(t)
	defer cleanup()

	cfg := Load([]string{})

	if cfg.SocialCards.Background != "#faf8f5" {
		t.Errorf("SocialCards.Background = %q, want %q", cfg.SocialCards.Background, "#faf8f5")
	}

	if len(cfg.SocialCards.Gradient) != 2 {
		t.Errorf("SocialCards.Gradient length = %d, want 2", len(cfg.SocialCards.Gradient))
	}

	if cfg.SocialCards.Angle != 135 {
		t.Errorf("SocialCards.Angle = %d, want 135", cfg.SocialCards.Angle)
	}

	if cfg.SocialCards.TextColor != "#1a1a1a" {
		t.Errorf("SocialCards.TextColor = %q, want %q", cfg.SocialCards.TextColor, "#1a1a1a")
	}
}

func TestConfig_FeaturesConfig(t *testing.T) {
	tests := []struct {
		name        string
		rawMarkdown bool
		expectRawMD bool
	}{
		{"raw markdown enabled", true, true},
		{"raw markdown disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := changeToTempDir(t)
			defer cleanup()

			yamlValue := "false"
			if tt.rawMarkdown {
				yamlValue = "true"
			}
			yamlContent := "features:\n  rawMarkdown: " + yamlValue
			if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to create test kosh.yaml: %v", err)
			}

			cfg := Load([]string{})

			if cfg.Features.RawMarkdown != tt.expectRawMD {
				t.Errorf("Features.RawMarkdown = %v, want %v", cfg.Features.RawMarkdown, tt.expectRawMD)
			}
		})
	}
}
