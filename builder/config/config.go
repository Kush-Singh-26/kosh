// Package config handles configuration loading and CLI overrides.
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

var _ models.TemplateConfig = (*Config)(nil)

const (
	DefaultPostsPerPage  = 10
	DefaultImageWorkers  = 8
	MaxImageWorkers      = 32
	DefaultWebPQuality   = 80
	MinWebPQuality       = 1
	MaxWebPQuality       = 100
	DefaultParserWorkers = 0
	MaxParserWorkers     = 64

	DefaultTheme      = "blog"
	DefaultThemeDir   = "themes"
	DefaultContentDir = "content"
	DefaultOutputDir  = "public"
	DefaultCacheDir   = ".kosh-cache"

	DefaultSocialCardAngle = 135
)

// ThemeConfig captures metadata about the active theme.
type ThemeConfig struct {
	Name string `yaml:"name"`
}

// SiteConfig defines site-level configuration.
type SiteConfig struct {
	Title       string              `yaml:"title"`
	Description string              `yaml:"description"`
	BaseURL     string              `yaml:"baseURL"`
	Language    string              `yaml:"language"`
	Author      models.AuthorConfig `yaml:"author"`
	Menu        []models.MenuEntry  `yaml:"menu"`
	FooterMenu  []models.MenuEntry  `yaml:"footerMenu"`
}

// BuildOptions defines build-time tuning parameters.
type BuildOptions struct {
	PostsPerPage         int  `yaml:"postsPerPage"`
	ShouldCompressImages bool `yaml:"shouldCompressImages"`
	ShouldMinifySVGs     bool `yaml:"shouldMinifySVGs"`
	ImageWorkers         int  `yaml:"imageWorkers"`  // Number of parallel image workers (default: 8)
	WebPQuality          int    `yaml:"webpQuality"`   // WebP image compression quality (1-100, default: 80)
	ParserWorkers        int    `yaml:"parserWorkers"` // Number of parallel parser workers (0 = auto, default: 0)
	BlogPrefix           string `yaml:"blogPrefix"`    // Prefix for blog-related output (default: "")
	NoStaging            bool   `yaml:"-"`             // Disable atomic staging
}

// ServerConfig defines development server parameters.
type ServerConfig struct {
	RootDirectory string `yaml:"rootDirectory"` // Optional directory to serve alongside the blog
}

// PathConfig defines filesystem paths used during builds.
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

// Config aggregates all site, build, and path configuration.
type Config struct {
	SiteConfig   `yaml:",inline"`
	BuildOptions `yaml:",inline"`
	PathConfig   `yaml:",inline"`

	Server        ServerConfig             `yaml:"server"`   // Server-specific configuration
	Features      models.FeaturesConfig    `yaml:"features"` // Enable/Disable features
	ThemeMetadata ThemeConfig              `yaml:"-"`        // Loaded from theme.yaml
	SocialCards   models.SocialCardsConfig `yaml:"socialCards"`

	// Internal / Runtime fields
	ShouldForceRebuild  bool   `yaml:"-"`
	ShouldForceLock     bool   `yaml:"-"`
	ShouldIncludeDrafts bool   `yaml:"-"`
	BuildVersion        int64  `yaml:"-"`
	IsDev               bool   `yaml:"-"`
	KoshSourceRoot      string `yaml:"-"` // Repository root for WASM compilation
	SiteRoot            string `yaml:"-"` // Working directory where kosh.yaml was loaded
	Debug               bool   `yaml:"-"` // Enable debug output

	// Build configuration (loaded from kosh.build.yaml)
	Build *BuildConfig `yaml:"-"`

	// SiteData holds structured data from the data/ directory
	SiteData map[string]any `yaml:"-"`
}

// GetSiteData returns the structured site data.
func (c *Config) GetSiteData() map[string]any {
	return c.SiteData
}

// Load loads configuration using the OS filesystem.
func Load(args []string) *Config {
	return LoadFs(afero.NewOsFs(), args)
}

func defaultConfig() *Config {
	return &Config{
		SiteConfig: SiteConfig{
			Title:   "Kosh Blog",
			BaseURL: "",
		},
		BuildOptions: BuildOptions{
			PostsPerPage:         DefaultPostsPerPage,
			ShouldCompressImages: true,
			ShouldMinifySVGs:     true,
			ImageWorkers:         DefaultImageWorkers,
			WebPQuality:          DefaultWebPQuality,
			ParserWorkers:        DefaultParserWorkers,
			BlogPrefix:           "",
			NoStaging:            true,
		},
		PathConfig: PathConfig{
			Theme:      DefaultTheme,
			ThemeDir:   DefaultThemeDir,
			ContentDir: DefaultContentDir,
			OutputDir:  DefaultOutputDir,
			CacheDir:   DefaultCacheDir,
		},
		BuildVersion: time.Now().Unix(),
		Features: models.FeaturesConfig{
			UseRawMarkdown: false,
			Generators: models.GeneratorsConfig{
				IsSitemapEnabled: true,
				IsRSSEnabled:     true,
				Graph:            models.GraphConfig{IsEnabled: true, ShowsTags: true},
				IsPWAEnabled:     true,
				IsSearchEnabled:  true,
			},
		},
		SocialCards: models.SocialCardsConfig{
			Background: "#faf8f5",
			Gradient:   []string{"#e8e0d0", "#d4c4a8"},
			Angle:      DefaultSocialCardAngle,
			TextColor:  "#1a1a1a",
		},
	}
}

func loadConfigFile(fs afero.Fs, cfg *Config) {
	if data, err := afero.ReadFile(fs, "kosh.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			slog.Warn("Failed to parse kosh.yaml", "error", err)
		}
		return
	}
	if data, err := afero.ReadFile(fs, "config.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			slog.Warn("Failed to parse config.yaml", "error", err)
		}
	}
}

func validateWorkerConfig(cfg *Config) {
	if cfg.ImageWorkers <= 0 {
		cfg.ImageWorkers = DefaultImageWorkers
	}
	if cfg.ImageWorkers > MaxImageWorkers {
		cfg.ImageWorkers = MaxImageWorkers
	}
	if cfg.ParserWorkers > MaxParserWorkers {
		cfg.ParserWorkers = MaxParserWorkers
	}
}

func resolveThemePaths(cfg *Config, isTesting bool) {
	if cfg.ThemeDir == "" {
		cfg.ThemeDir = DefaultThemeDir
	}
	if !isTesting {
		if absPath, err := filepath.Abs(cfg.ThemeDir); err == nil {
			cfg.ThemeDir = fspkg.NormalizePath(absPath)
		}
	}

	if cfg.TemplateDir == "" {
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	} else if !filepath.IsAbs(cfg.TemplateDir) && !isTesting {
		if absPath, err := filepath.Abs(cfg.TemplateDir); err == nil {
			cfg.TemplateDir = fspkg.NormalizePath(absPath)
		}
	} else {
		cfg.TemplateDir = fspkg.NormalizePath(cfg.TemplateDir)
	}

	if cfg.StaticDir == "" {
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	} else if !filepath.IsAbs(cfg.StaticDir) && !isTesting {
		if absPath, err := filepath.Abs(cfg.StaticDir); err == nil {
			cfg.StaticDir = fspkg.NormalizePath(absPath)
		}
	} else {
		cfg.StaticDir = fspkg.NormalizePath(cfg.StaticDir)
	}
}

func resolveContentPaths(cfg *Config, isTesting bool) {
	if cfg.ContentDir == "" {
		cfg.ContentDir = DefaultContentDir
	}
	if !isTesting {
		if absPath, err := filepath.Abs(cfg.ContentDir); err == nil {
			cfg.ContentDir = fspkg.NormalizePath(absPath)
		}
	}

	if cfg.OutputDir == "" {
		cfg.OutputDir = DefaultOutputDir
	}
	if !isTesting {
		if absPath, err := filepath.Abs(cfg.OutputDir); err == nil {
			cfg.OutputDir = fspkg.NormalizePath(absPath)
		}
	}

	if cfg.CacheDir == "" {
		cfg.CacheDir = DefaultCacheDir
	}
	if !isTesting {
		if absPath, err := filepath.Abs(cfg.CacheDir); err == nil {
			cfg.CacheDir = fspkg.NormalizePath(absPath)
		}
	}
}

func applyCLIOverrides(cfg *Config, args []string) {
	flagSet := flag.NewFlagSet("config", flag.ContinueOnError)
	baseUrlFlag := flagSet.String("baseurl", "", "Base URL (overrides config file)")
	draftsFlag := flagSet.Bool("drafts", false, "Include draft posts in the build")
	themeFlag := flagSet.String("theme", "", "Theme to use (overrides config file)")
	forceLockFlag := flagSet.Bool("force-lock", false, "Acquire build lock even if another build is running")
	debugFlag := flagSet.Bool("debug", false, "Enable debug output")
	staging := flagSet.Bool("staging", false, "Use atomic staging for build (disables direct output writing)")
	noStaging := flagSet.Bool("no-staging", true, "Disable atomic staging (overwrites output in place)")

	_ = flagSet.Parse(args)

	if *baseUrlFlag != "" {
		cfg.BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	}
	if *draftsFlag {
		cfg.ShouldIncludeDrafts = true
	}
	if *forceLockFlag {
		cfg.ShouldForceLock = true
	}
	if *themeFlag != "" {
		cfg.Theme = *themeFlag
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	}
	if *debugFlag {
		cfg.Build.Debug = true
	}
	if *staging {
		cfg.NoStaging = false
	} else {
		cfg.NoStaging = *noStaging
	}
}

func finalizeConfig(cfg *Config) {
	if cfg.WebPQuality < MinWebPQuality || cfg.WebPQuality > MaxWebPQuality {
		cfg.WebPQuality = DefaultWebPQuality
	}

	cfg.KoshSourceRoot = os.Getenv("KOSH_REPO_ROOT")
	if cfg.KoshSourceRoot == "" {
		cfg.KoshSourceRoot = fspkg.RepoRoot()
	}

	if wd, err := os.Getwd(); err == nil {
		cfg.SiteRoot = fspkg.NormalizePath(wd)
	} else {
		cfg.SiteRoot = "."
	}

	if cfg.Server.RootDirectory != "" && !filepath.IsAbs(cfg.Server.RootDirectory) {
		cfg.Server.RootDirectory = filepath.Join(cfg.SiteRoot, cfg.Server.RootDirectory)
		cfg.Server.RootDirectory = fspkg.NormalizePath(cfg.Server.RootDirectory)
	}
}

// LoadFs loads configuration using the provided filesystem.
func LoadFs(fs afero.Fs, args []string) *Config {
	cfg := defaultConfig()

	// 2. Load from YAML file if exists
	loadConfigFile(fs, cfg)

	// Validate and set defaults for ImageWorkers
	validateWorkerConfig(cfg)

	// Load build configuration from kosh.build.yaml
	cfg.Build = LoadBuildConfigFs(fs)

	isTesting := fspkg.DetectTestingMode()

	// 3. Apply Smart Defaults and resolve to absolute paths
	resolveThemePaths(cfg, isTesting)
	resolveContentPaths(cfg, isTesting)

	// 3. Override with CLI Flags
	applyCLIOverrides(cfg, args)
	finalizeConfig(cfg)

	return cfg
}

// SetDevMode toggles dev mode on the config.
func SetDevMode(cfg *Config, isDev bool) {
	cfg.IsDev = isDev
}

// TemplateConfig interface implementation

// GetMenu returns the configured menu entries.
func (cfg *Config) GetMenu() []models.MenuEntry { return cfg.Menu }

// GetFooterMenu returns the configured footer menu entries.
func (cfg *Config) GetFooterMenu() []models.MenuEntry { return cfg.FooterMenu }

// GetAuthor returns the configured author metadata.
func (cfg *Config) GetAuthor() models.AuthorConfig { return cfg.Author }

// GetSocial returns the social card configuration.
func (cfg *Config) GetSocial() models.SocialCardsConfig { return cfg.SocialCards }

// GetFeatures returns the enabled feature configuration.
func (cfg *Config) GetFeatures() models.FeaturesConfig { return cfg.Features }

// GetSiteTitle returns the site title.
func (cfg *Config) GetSiteTitle() string { return cfg.Title }

// GetLogo returns the path to the site logo.
func (cfg *Config) GetLogo() string { return cfg.Logo }

// GetBaseURL returns the configured base URL.
func (cfg *Config) GetBaseURL() string { return cfg.BaseURL }

// GetBlogPrefix returns the blog prefix path.
func (cfg *Config) GetBlogPrefix() string { return cfg.BlogPrefix }

// IsDevMode returns whether the build is running in development mode.
func (cfg *Config) IsDevMode() bool { return cfg.IsDev }
