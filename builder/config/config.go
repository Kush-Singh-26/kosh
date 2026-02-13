// handles command-line flags
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"my-ssg/builder/models"
	"my-ssg/builder/utils"

	"gopkg.in/yaml.v3"
)

// Global flag to track if we're in development mode
var isDevMode = atomic.Bool{}

type MenuEntry struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url,omitempty"`
	Target string `yaml:"target,omitempty"`
	ID     string `yaml:"id,omitempty"`
	Class  string `yaml:"class,omitempty"`
}

// Version represents a documentation version
type Version struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path"` // "" for latest, "v2.0", "v1.0", etc.
	IsLatest bool   `yaml:"isLatest"`
	Strategy string `yaml:"strategy"` // "snapshot" or "delta"
}

type GeneratorsConfig struct {
	Sitemap bool `yaml:"sitemap"`
	RSS     bool `yaml:"rss"`
	Graph   bool `yaml:"graph"`
	PWA     bool `yaml:"pwa"`
	Search  bool `yaml:"search"`
}

type FeaturesConfig struct {
	RawMarkdown bool             `yaml:"rawMarkdown"`
	Generators  GeneratorsConfig `yaml:"generators"`
}

type AuthorConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ThemeConfig struct {
	Name               string `yaml:"name"`
	SupportsVersioning bool   `yaml:"supportsVersioning"`
}

type SocialCardsConfig struct {
	Background string   `yaml:"background"`
	Gradient   []string `yaml:"gradient"`
	Angle      int      `yaml:"angle"`
	TextColor  string   `yaml:"textColor"`
}

type Config struct {
	Title          string            `yaml:"title"`
	Description    string            `yaml:"description"`
	BaseURL        string            `yaml:"baseURL"`
	Language       string            `yaml:"language"`
	Author         AuthorConfig      `yaml:"author"`
	Menu           []MenuEntry       `yaml:"menu"`
	PostsPerPage   int               `yaml:"postsPerPage"`
	CompressImages bool              `yaml:"compressImages"`
	ImageWorkers   int               `yaml:"imageWorkers"` // Number of parallel image workers (default: 24)
	Theme          string            `yaml:"theme"`
	ThemeDir       string            `yaml:"themeDir"`
	TemplateDir    string            `yaml:"templateDir"`
	StaticDir      string            `yaml:"staticDir"`
	Logo           string            `yaml:"logo"`     // Path to site logo/favicon
	Versions       []Version         `yaml:"versions"` // Documentation versions
	Features       FeaturesConfig    `yaml:"features"` // Enable/Disable features
	ThemeMetadata  ThemeConfig       `yaml:"-"`        // Loaded from theme.yaml
	SocialCards    SocialCardsConfig `yaml:"socialCards"`

	// Configurable directory paths
	ContentDir string `yaml:"contentDir"` // Content source directory (default: "content")
	OutputDir  string `yaml:"outputDir"`  // Build output directory (default: "public")
	CacheDir   string `yaml:"cacheDir"`   // Cache directory (default: ".kosh-cache")

	// Internal / Runtime fields
	ForceRebuild  bool  `yaml:"-"`
	IncludeDrafts bool  `yaml:"-"`
	BuildVersion  int64 `yaml:"-"`
	IsDev         bool  `yaml:"-"`
}

func Load(args []string) *Config {
	// 1. Default Configuration
	cfg := &Config{
		Title:          "Kosh Blog",
		BaseURL:        "",
		PostsPerPage:   10,
		CompressImages: true, // Always compress for performance
		ImageWorkers:   24,   // Default 24 parallel workers for image processing
		BuildVersion:   time.Now().Unix(),
		Theme:          "blog",
		ThemeDir:       "themes",
		ContentDir:     "content",
		OutputDir:      "public",
		CacheDir:       ".kosh-cache",
		Features: FeaturesConfig{
			RawMarkdown: false,
			Generators: GeneratorsConfig{
				Sitemap: true,
				RSS:     true,
				Graph:   true,
				PWA:     true,
				Search:  true,
			},
		},
		SocialCards: SocialCardsConfig{
			Background: "#faf8f5",
			Gradient:   []string{"#e8e0d0", "#d4c4a8"},
			Angle:      135,
			TextColor:  "#1a1a1a",
		},
	}

	// 2. Load from YAML file if exists
	if data, err := os.ReadFile("kosh.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			fmt.Printf("⚠️ Failed to parse kosh.yaml: %v\n", err)
		}
	} else {
		// Try fallback to config.yaml
		if data, err := os.ReadFile("config.yaml"); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				fmt.Printf("⚠️ Failed to parse config.yaml: %v\n", err)
			}
		}
	}

	// Validate and set defaults for ImageWorkers
	if cfg.ImageWorkers <= 0 {
		cfg.ImageWorkers = 24
	}
	// Cap at reasonable maximum to prevent resource exhaustion
	if cfg.ImageWorkers > 32 {
		cfg.ImageWorkers = 32
	}

	// 3. Apply Smart Defaults and resolve to absolute paths
	if cfg.ThemeDir == "" {
		cfg.ThemeDir = "themes"
	}
	if abs, err := filepath.Abs(cfg.ThemeDir); err == nil {
		cfg.ThemeDir = utils.NormalizePath(abs)
	}

	if cfg.TemplateDir == "" {
		// Default: themes/<theme>/templates
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	} else if !filepath.IsAbs(cfg.TemplateDir) {
		if abs, err := filepath.Abs(cfg.TemplateDir); err == nil {
			cfg.TemplateDir = utils.NormalizePath(abs)
		}
	} else {
		cfg.TemplateDir = utils.NormalizePath(cfg.TemplateDir)
	}

	if cfg.StaticDir == "" {
		// Default: themes/<theme>/static
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	} else if !filepath.IsAbs(cfg.StaticDir) {
		if abs, err := filepath.Abs(cfg.StaticDir); err == nil {
			cfg.StaticDir = utils.NormalizePath(abs)
		}
	} else {
		cfg.StaticDir = utils.NormalizePath(cfg.StaticDir)
	}

	// Resolve configurable directory paths to absolute paths
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content"
	}
	if abs, err := filepath.Abs(cfg.ContentDir); err == nil {
		cfg.ContentDir = utils.NormalizePath(abs)
	}

	if cfg.OutputDir == "" {
		cfg.OutputDir = "public"
	}
	if abs, err := filepath.Abs(cfg.OutputDir); err == nil {
		cfg.OutputDir = utils.NormalizePath(abs)
	}

	if cfg.CacheDir == "" {
		cfg.CacheDir = ".kosh-cache"
	}
	if abs, err := filepath.Abs(cfg.CacheDir); err == nil {
		cfg.CacheDir = utils.NormalizePath(abs)
	}

	// 3. Override with CLI Flags
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	baseUrlFlag := fs.String("baseurl", "", "Base URL (overrides config file)")
	draftsFlag := fs.Bool("drafts", false, "Include draft posts in the build")
	themeFlag := fs.String("theme", "", "Theme to use (overrides config file)")

	_ = fs.Parse(args)

	if *baseUrlFlag != "" {
		cfg.BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	}
	if *draftsFlag {
		cfg.IncludeDrafts = true
	}
	if *themeFlag != "" {
		cfg.Theme = *themeFlag
		// Re-apply smart defaults and absolute resolution since theme changed
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	}

	return cfg
}

// SetDevMode is a helper to set development mode on a config pointer
func SetDevMode(cfg *Config, isDev bool) {
	cfg.IsDev = isDev
	isDevMode.Store(isDev)
}

// GetVersionsMetadata returns a list of version information for templates
func (cfg *Config) GetVersionsMetadata(currentVersion string) []models.VersionInfo {
	if len(cfg.Versions) == 0 {
		return nil
	}

	var results []models.VersionInfo
	for _, v := range cfg.Versions {
		url := utils.BuildURL(cfg.BaseURL, v.Path, "")

		results = append(results, models.VersionInfo{
			Name:      v.Name,
			URL:       url,
			IsLatest:  v.IsLatest,
			IsCurrent: v.Path == currentVersion,
		})
	}
	return results
}
