// handles command-line flags
package config

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/spf13/afero"

	"gopkg.in/yaml.v3"
)

type ThemeConfig struct {
	Name string `yaml:"name"`
}

type SiteConfig struct {
	Title       string              `yaml:"title"`
	Description string              `yaml:"description"`
	BaseURL     string              `yaml:"baseURL"`
	Language    string              `yaml:"language"`
	Author      models.AuthorConfig `yaml:"author"`
	Menu        []models.MenuEntry  `yaml:"menu"`
}

type BuildOptions struct {
	PostsPerPage   int  `yaml:"postsPerPage"`
	CompressImages bool `yaml:"compressImages"`
	MinifySVGs     bool `yaml:"minifySVGs"`
	ImageWorkers   int  `yaml:"imageWorkers"`  // Number of parallel image workers (default: 8)
	WebPQuality    int  `yaml:"webpQuality"`   // WebP image compression quality (1-100, default: 80)
	ParserWorkers  int  `yaml:"parserWorkers"` // Number of parallel parser workers (0 = auto, default: 0)
}

type PathConfig struct {
	Theme       string `yaml:"theme"`
	ThemeDir    string `yaml:"themeDir"`
	TemplateDir string `yaml:"templateDir"`
	StaticDir   string `yaml:"staticDir"`
	Logo        string `yaml:"logo"`       // Path to site logo (unified source for branding, PWA icons, etc.)
	ContentDir  string `yaml:"contentDir"` // Content source directory (default: "content")
	OutputDir   string `yaml:"outputDir"`  // Build output directory (default: "public")
	CacheDir    string `yaml:"cacheDir"`   // Cache directory (default: ".kosh-cache")
}

type Config struct {
	SiteConfig   `yaml:",inline"`
	BuildOptions `yaml:",inline"`
	PathConfig   `yaml:",inline"`

	Features      models.FeaturesConfig    `yaml:"features"` // Enable/Disable features
	ThemeMetadata ThemeConfig              `yaml:"-"`        // Loaded from theme.yaml
	SocialCards   models.SocialCardsConfig `yaml:"socialCards"`

	// Internal / Runtime fields
	ForceRebuild   bool   `yaml:"-"`
	ForceLock      bool   `yaml:"-"`
	IncludeDrafts  bool   `yaml:"-"`
	BuildVersion   int64  `yaml:"-"`
	IsDev          bool   `yaml:"-"`
	KoshSourceRoot string `yaml:"-"` // Repository root for WASM compilation
	SiteRoot       string `yaml:"-"` // Working directory where kosh.yaml was loaded

	// Build configuration (loaded from kosh.build.yaml)
	Build *BuildConfig `yaml:"-"`
}

func Load(args []string) *Config {
	return LoadFs(afero.NewOsFs(), args)
}

func LoadFs(fs afero.Fs, args []string) *Config {
	// 1. Default Configuration
	cfg := &Config{
		SiteConfig: SiteConfig{
			Title:   "Kosh Blog",
			BaseURL: "",
		},
		BuildOptions: BuildOptions{
			PostsPerPage:   10,
			CompressImages: true, // Always compress for performance
			MinifySVGs:     true,
			ImageWorkers:   8,  // Default 8 parallel workers for image processing (benchmarked optimum)
			WebPQuality:    80, // Default WebP quality is 80
			ParserWorkers:  0,  // 0 = auto (use models.GetDefaultWorkerCount)
		},
		PathConfig: PathConfig{
			Theme:      "blog",
			ThemeDir:   "themes",
			ContentDir: "content",
			OutputDir:  "public",
			CacheDir:   ".kosh-cache",
		},
		BuildVersion: time.Now().Unix(),
		Features: models.FeaturesConfig{
			RawMarkdown: false,
			Generators: models.GeneratorsConfig{
				Sitemap: true,
				RSS:     true,
				Graph:   models.GraphConfig{Enabled: true, ShowTags: true},
				PWA:     true,
				Search:  true,
			},
		},
		SocialCards: models.SocialCardsConfig{
			Background: "#faf8f5",
			Gradient:   []string{"#e8e0d0", "#d4c4a8"},
			Angle:      135,
			TextColor:  "#1a1a1a",
		},
	}

	// 2. Load from YAML file if exists
	if data, err := afero.ReadFile(fs, "kosh.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			slog.Warn("Failed to parse kosh.yaml", "error", err)
		}
	} else {
		// Try fallback to config.yaml
		if data, err := afero.ReadFile(fs, "config.yaml"); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				slog.Warn("Failed to parse config.yaml", "error", err)
			}
		}
	}

	// Validate and set defaults for ImageWorkers
	if cfg.ImageWorkers <= 0 {
		cfg.ImageWorkers = 8
	}
	// Cap at reasonable maximum to prevent resource exhaustion
	if cfg.ImageWorkers > 32 {
		cfg.ImageWorkers = 32
	}

	// Validate ParserWorkers (0 = auto)
	if cfg.ParserWorkers > 64 {
		cfg.ParserWorkers = 64
	}

	// Load build configuration from kosh.build.yaml
	cfg.Build = LoadBuildConfigFs(fs)

	isTesting := fspkg.DetectTestingMode()

	// 3. Apply Smart Defaults and resolve to absolute paths
	if cfg.ThemeDir == "" {
		cfg.ThemeDir = "themes"
	}
	if !isTesting {
		if abs, err := filepath.Abs(cfg.ThemeDir); err == nil {
			cfg.ThemeDir = fspkg.NormalizePath(abs)
		}
	}

	if cfg.TemplateDir == "" {
		// Default: themes/<theme>/templates
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	} else if !filepath.IsAbs(cfg.TemplateDir) && !isTesting {
		if abs, err := filepath.Abs(cfg.TemplateDir); err == nil {
			cfg.TemplateDir = fspkg.NormalizePath(abs)
		}
	} else {
		cfg.TemplateDir = fspkg.NormalizePath(cfg.TemplateDir)
	}

	if cfg.StaticDir == "" {
		// Default: themes/<theme>/static
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	} else if !filepath.IsAbs(cfg.StaticDir) && !isTesting {
		if abs, err := filepath.Abs(cfg.StaticDir); err == nil {
			cfg.StaticDir = fspkg.NormalizePath(abs)
		}
	} else {
		cfg.StaticDir = fspkg.NormalizePath(cfg.StaticDir)
	}

	// Resolve configurable directory paths to absolute paths
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content"
	}
	if !isTesting {
		if abs, err := filepath.Abs(cfg.ContentDir); err == nil {
			cfg.ContentDir = fspkg.NormalizePath(abs)
		}
	}

	if cfg.OutputDir == "" {
		cfg.OutputDir = "public"
	}
	if !isTesting {
		if abs, err := filepath.Abs(cfg.OutputDir); err == nil {
			cfg.OutputDir = fspkg.NormalizePath(abs)
		}
	}

	if cfg.CacheDir == "" {
		cfg.CacheDir = ".kosh-cache"
	}
	if !isTesting {
		if abs, err := filepath.Abs(cfg.CacheDir); err == nil {
			cfg.CacheDir = fspkg.NormalizePath(abs)
		}
	}

	// 3. Override with CLI Flags
	fset := flag.NewFlagSet("config", flag.ContinueOnError)
	baseUrlFlag := fset.String("baseurl", "", "Base URL (overrides config file)")
	draftsFlag := fset.Bool("drafts", false, "Include draft posts in the build")
	themeFlag := fset.String("theme", "", "Theme to use (overrides config file)")
	forceLockFlag := fset.Bool("force-lock", false, "Acquire build lock even if another build is running")

	_ = fset.Parse(args)

	if *baseUrlFlag != "" {
		cfg.BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	}
	if *draftsFlag {
		cfg.IncludeDrafts = true
	}
	if *forceLockFlag {
		cfg.ForceLock = true
	}
	if *themeFlag != "" {
		cfg.Theme = *themeFlag
		// Re-apply smart defaults and absolute resolution since theme changed
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	}

	if cfg.WebPQuality < 1 || cfg.WebPQuality > 100 {
		cfg.WebPQuality = 80 // enforce valid range
	}

	// Set repository root for WASM compilation and source lookups
	cfg.KoshSourceRoot = os.Getenv("KOSH_REPO_ROOT")
	if cfg.KoshSourceRoot == "" {
		cfg.KoshSourceRoot = fspkg.RepoRoot()
	}

	// Capture working directory (site root) for resolving site-level paths like ./static
	if wd, err := os.Getwd(); err == nil {
		cfg.SiteRoot = fspkg.NormalizePath(wd)
	} else {
		cfg.SiteRoot = "."
	}

	return cfg
}

func SetDevMode(cfg *Config, isDev bool) {
	cfg.IsDev = isDev
}

// TemplateConfig interface implementation

func (cfg *Config) GetMenu() []models.MenuEntry         { return cfg.Menu }
func (cfg *Config) GetAuthor() models.AuthorConfig      { return cfg.Author }
func (cfg *Config) GetSocial() models.SocialCardsConfig { return cfg.SocialCards }
func (cfg *Config) GetFeatures() models.FeaturesConfig  { return cfg.Features }
func (cfg *Config) GetSiteTitle() string                { return cfg.Title }
func (cfg *Config) GetBaseURL() string                  { return cfg.BaseURL }
