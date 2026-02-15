package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/utils"
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

	if cfg.ImageWorkers != 24 {
		t.Errorf("ImageWorkers = %d, want 24", cfg.ImageWorkers)
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

	if !cfg.Features.Generators.Graph {
		t.Error("Graph generator should be enabled by default")
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
		name     string
		workers  int
		expected int
	}{
		{"zero defaults to 24", 0, 24},
		{"negative defaults to 24", -1, 24},
		{"valid value", 16, 16},
		{"maximum cap", 50, 32},
		{"at maximum", 32, 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := changeToTempDir(t)
			defer cleanup()

			yamlContent := ""
			if tt.workers != 0 {
				yamlContent = fmt.Sprintf("imageWorkers: %d", tt.workers)
			}
			if err := os.WriteFile("kosh.yaml", []byte(yamlContent), 0644); err != nil {
				t.Fatalf("Failed to create test kosh.yaml: %v", err)
			}

			cfg := Load([]string{})

			if cfg.ImageWorkers != tt.expected {
				t.Errorf("ImageWorkers = %d, want %d", cfg.ImageWorkers, tt.expected)
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

func TestGetVersionsMetadata(t *testing.T) {
	tests := []struct {
		name                string
		versions            []Version
		currentVersion      string
		currentPath         string
		expectedCount       int
		expectedLatest      bool
		expectedCurrent     bool
		expectedLatestLabel string
	}{
		{
			name:          "no versions",
			versions:      []Version{},
			currentPath:   "",
			expectedCount: 0,
		},
		{
			name: "single version",
			versions: []Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: true},
			},
			currentVersion:      "v1.0",
			currentPath:         "getting-started.html",
			expectedCount:       1,
			expectedLatest:      true,
			expectedCurrent:     true,
			expectedLatestLabel: "v1.0 (Latest)",
		},
		{
			name: "multiple versions",
			versions: []Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: false},
				{Name: "v2.0", Path: "v2.0", IsLatest: true},
			},
			currentVersion:      "v2.0",
			currentPath:         "getting-started.html",
			expectedCount:       2,
			expectedLatest:      true,
			expectedCurrent:     true,
			expectedLatestLabel: "v2.0 (Latest)",
		},
		{
			name: "latest version with empty path",
			versions: []Version{
				{Name: "v4.0", Path: "", IsLatest: true},
				{Name: "v3.0", Path: "v3.0", IsLatest: false},
			},
			currentVersion:      "",
			currentPath:         "getting-started.html",
			expectedCount:       2,
			expectedLatest:      true,
			expectedCurrent:     true,
			expectedLatestLabel: "v4.0 (Latest)",
		},
		{
			name: "no latest flag",
			versions: []Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: false},
				{Name: "v2.0", Path: "v2.0", IsLatest: false},
			},
			currentVersion:  "v2.0",
			currentPath:     "getting-started.html",
			expectedCount:   2,
			expectedLatest:  false,
			expectedCurrent: true,
		},
		{
			name: "path with version prefix gets cleaned",
			versions: []Version{
				{Name: "v4.0", Path: "", IsLatest: true},
				{Name: "v3.0", Path: "v3.0", IsLatest: false},
			},
			currentVersion:  "v3.0",
			currentPath:     "v3.0/getting-started.html",
			expectedCount:   2,
			expectedLatest:  true,
			expectedCurrent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				BaseURL:  "https://example.com",
				Versions: tt.versions,
			}

			results := cfg.GetVersionsMetadata(tt.currentVersion, tt.currentPath)

			if len(results) != tt.expectedCount {
				t.Errorf("GetVersionsMetadata() returned %d results, want %d", len(results), tt.expectedCount)
				return
			}

			if tt.expectedCount > 0 {
				// Check that URLs are built correctly with path preservation
				for i, v := range cfg.Versions {
					// Expected path should be the clean path (without version prefix)
					expectedPath := tt.currentPath
					// Clean version prefix from path
					if tt.currentVersion != "" && expectedPath != "" {
						prefix := tt.currentVersion + "/"
						expectedPath = strings.TrimPrefix(expectedPath, prefix)
						prefixLower := strings.ToLower(tt.currentVersion) + "/"
						expectedPath = strings.TrimPrefix(expectedPath, prefixLower)
					}
					expectedURL := utils.BuildURL(cfg.BaseURL, v.Path, expectedPath)
					if results[i].URL != expectedURL {
						t.Errorf("Version %d URL = %q, want %q", i, results[i].URL, expectedURL)
					}
				}

				// Check latest flag
				if tt.expectedLatest && !results[0].IsLatest && !results[len(results)-1].IsLatest {
					t.Error("Expected one version to be marked as latest")
				}

				// Check current flag
				foundCurrent := false
				for _, r := range results {
					if r.IsCurrent {
						foundCurrent = true
						break
					}
				}
				if tt.expectedCurrent && !foundCurrent {
					t.Error("Expected to find current version")
				}

				// Check latest label
				if tt.expectedLatestLabel != "" {
					foundLatest := false
					for _, r := range results {
						if r.IsLatest && r.Name == tt.expectedLatestLabel {
							foundLatest = true
							break
						}
					}
					if !foundLatest {
						t.Errorf("Expected latest version name to be %q", tt.expectedLatestLabel)
					}
				}
			}
		})
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
