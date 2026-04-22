// Package config handles configuration loading and CLI overrides.
package config

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/spf13/afero"

	"gopkg.in/yaml.v3"
)

var _ models.TemplateConfig = (*Config)(nil)

const (
	// DefaultItemsPerPage is the default number of items per pagination page.
	DefaultItemsPerPage = 10
	// DefaultImageWorkers is the default number of image processing workers.
	DefaultImageWorkers = 8
	// MaxImageWorkers is the maximum number of image processing workers.
	MaxImageWorkers = 32
	// DefaultWebPQuality is the default WebP encoding quality.
	DefaultWebPQuality = 80
	// MinWebPQuality is the minimum WebP quality value.
	MinWebPQuality = 1
	// MaxWebPQuality is the maximum WebP quality value.
	MaxWebPQuality = 100
	// DefaultParserWorkers is the default number of markdown parser workers.
	DefaultParserWorkers = 0
	// MaxParserWorkers is the maximum number of parser workers.
	MaxParserWorkers = 64

	// DefaultTheme is the default theme name.
	DefaultTheme = "default"
	// DefaultThemeDir is the default theme directory.
	DefaultThemeDir = "themes"
	// DefaultContentDir is the default content directory.
	DefaultContentDir = "content"
	// DefaultOutputDir is the default output directory.
	DefaultOutputDir = "public"
	// DefaultCacheDir is the default cache directory.
	DefaultCacheDir = ".kosh-cache"
	// DefaultLayoutsDir is the default layouts directory.
	DefaultLayoutsDir = "layouts"

	// DefaultSocialCardAngle is the default social card gradient angle.
	DefaultSocialCardAngle = 135
)

// ThemeConfig captures metadata about the active theme.
type ThemeConfig struct {
	Name string `yaml:"name"`
}

// SiteConfig defines site-level configuration.
type SiteConfig struct {
	Title       string                      `yaml:"title"`
	Description string                      `yaml:"description"`
	BaseURL     string                      `yaml:"baseURL"`
	Language    string                      `yaml:"language"`
	Author      models.AuthorConfig         `yaml:"author"`
	Menu        []models.MenuEntry          `yaml:"menu"`
	FooterMenu  []models.MenuEntry          `yaml:"footerMenu"`
	Taxonomies  map[string]string           `yaml:"taxonomies"`  // Maps frontmatter key to plural folder name
	Navbar      models.NavbarIdentityConfig `yaml:"navbar"`      // Context-aware branding
	HomeBadge   string                      `yaml:"homeBadge"`   // Label for home page social card badge
	ArticleType string                      `yaml:"articleType"` // Schema.org article type (default: "BlogPosting")
}

// BuildOptions defines build-time tuning parameters.
type BuildOptions struct {
	ItemsPerPage         int    `yaml:"itemsPerPage"` // Number of items per pagination page
	ShouldCompressImages bool   `yaml:"shouldCompressImages"`
	ShouldMinify         bool   `yaml:"shouldMinify"`
	ImageWorkers         int    `yaml:"imageWorkers"`  // Number of parallel image workers (default: 8)
	WebPQuality          int    `yaml:"webpQuality"`   // WebP image compression quality (1-100, default: 80)
	ParserWorkers        int    `yaml:"parserWorkers"` // Number of parallel parser workers (0 = auto, default: 0)
	ContentPrefix        string `yaml:"contentPrefix"` // Prefix for content-related output (default: "")
	NoStaging            bool   `yaml:"-"`             // Disable atomic staging
}

// ServerConfig defines development server parameters.
type ServerConfig struct {
	RootDirectory string `yaml:"rootDirectory"` // Optional directory to serve alongside the site content
}

// PathConfig defines filesystem paths used during builds.
type PathConfig struct {
	Theme       string `yaml:"theme"`
	ThemeDir    string `yaml:"themeDir"`
	TemplateDir string `yaml:"templateDir"`
	StaticDir   string `yaml:"staticDir"`
	Logo        string `yaml:"logo"`       // Path to site logo (unified source for branding, PWA icons, etc.)
	Icon192     string `yaml:"icon192"`    // Path to 192x192 PWA icon (optional, defaults to generated from Logo)
	Icon512     string `yaml:"icon512"`    // Path to 512x512 PWA icon (optional, defaults to generated from Logo)
	ContentDir  string `yaml:"contentDir"` // Content source directory (default: "content")
	LayoutsDir  string `yaml:"layoutsDir"` // Site-level template overrides (default: "layouts")
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
		SiteConfig:   defaultSiteConfig(),
		BuildOptions: defaultBuildOptions(),
		PathConfig:   defaultPathConfig(),
		Features:     defaultFeaturesConfig(),
		SocialCards:  defaultSocialCardsConfig(),
		BuildVersion: 0,
	}
}

func defaultSiteConfig() SiteConfig {
	return SiteConfig{
		Title:   "Kosh Site",
		BaseURL: "",
		Taxonomies: map[string]string{
			"tags":   "tags",
			"series": "series",
			"events": "events",
		},
		Navbar: models.NavbarIdentityConfig{
			Home:    models.NavbarContextConfig{Title: "Kosh Site", BtnLabel: "Content"},
			Section: models.NavbarContextConfig{Title: "Content", BtnLabel: "Home"},
		},
		HomeBadge:   "Latest Items",
		ArticleType: "BlogPosting",
	}
}

func defaultBuildOptions() BuildOptions {
	return BuildOptions{
		ItemsPerPage:         DefaultItemsPerPage,
		ShouldCompressImages: true,
		ShouldMinify:         true,
		ImageWorkers:         DefaultImageWorkers,
		WebPQuality:          DefaultWebPQuality,
		ParserWorkers:        DefaultParserWorkers,
		ContentPrefix:        "",
		NoStaging:            true,
	}
}

func defaultPathConfig() PathConfig {
	return PathConfig{
		Theme:      DefaultTheme,
		ThemeDir:   DefaultThemeDir,
		ContentDir: DefaultContentDir,
		OutputDir:  DefaultOutputDir,
		CacheDir:   DefaultCacheDir,
	}
}

func defaultFeaturesConfig() models.FeaturesConfig {
	return models.FeaturesConfig{
		UseRawMarkdown: false,
		Generators: models.GeneratorsConfig{
			IsSitemapEnabled: true,
			IsRSSEnabled:     true,
			Graph:            models.GraphConfig{IsEnabled: true, ShowsTaxonomies: true},
			IsPWAEnabled:     true,
			Search: models.SearchOptionsConfig{
				IsEnabled: true,
				Ranking: models.SearchRankingConfig{
					TitleBoost:       50.0,
					TagBoost:         5.0,
					DescriptionBoost: 5.0,
					BM25K1:           1.2,
					BM25B:            0.75,
				},
				Endpoints: nil,
			},
		},
	}
}

func defaultSocialCardsConfig() models.SocialCardsConfig {
	return models.SocialCardsConfig{
		Background: "#141816", // Deep Inky Black
		Gradient:   nil,       // Classy solid look
		Angle:      DefaultSocialCardAngle,
		TextColor:  "#e1e9e4", // Scholar's Slate
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

	switch {
	case cfg.TemplateDir == "":
		cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	case !filepath.IsAbs(cfg.TemplateDir) && !isTesting:
		if absPath, err := filepath.Abs(cfg.TemplateDir); err == nil {
			cfg.TemplateDir = fspkg.NormalizePath(absPath)
		}
	default:
		cfg.TemplateDir = fspkg.NormalizePath(cfg.TemplateDir)
	}

	switch {
	case cfg.StaticDir == "":
		cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	case !filepath.IsAbs(cfg.StaticDir) && !isTesting:
		if absPath, err := filepath.Abs(cfg.StaticDir); err == nil {
			cfg.StaticDir = fspkg.NormalizePath(absPath)
		}
	default:
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

	if cfg.LayoutsDir == "" {
		cfg.LayoutsDir = DefaultLayoutsDir
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
	baseURLFlag := flagSet.String("baseurl", "", "Base URL (overrides config file)")
	draftsFlag := flagSet.Bool("drafts", false, "Include draft posts in the build")
	themeFlag := flagSet.String("theme", "", "Theme to use (overrides config file)")
	forceLockFlag := flagSet.Bool("force-lock", false, "Acquire build lock even if another build is running")
	debugFlag := flagSet.Bool("debug", false, "Enable debug output")
	staging := flagSet.Bool("staging", false, "Use atomic staging for build (disables direct output writing)")
	noStaging := flagSet.Bool("no-staging", true, "Disable atomic staging (overwrites output in place)")

	_ = flagSet.Parse(args)

	if *baseURLFlag != "" {
		cfg.BaseURL = strings.TrimSuffix(*baseURLFlag, "/")
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

// Reload reloads the configuration from disk into the existing instance.
func (c *Config) Reload(fs afero.Fs) error {
	// 1. Re-load from YAML file
	loadConfigFile(fs, c)

	// 2. Validate and set defaults for ImageWorkers
	validateWorkerConfig(c)

	// 3. Re-load build configuration
	c.Build = LoadBuildConfigFs(fs)

	// 4. Re-resolve paths to absolute paths (critical for dev mode)
	isTesting := fspkg.DetectTestingMode()
	resolveThemePaths(c, isTesting)
	resolveContentPaths(c, isTesting)

	// 5. Finalize
	finalizeConfig(c)

	return nil
}

// SetDevMode toggles dev mode on the config.
func SetDevMode(cfg *Config, isDev bool) {
	cfg.IsDev = isDev
}

// TemplateConfig interface implementation

// GetMenu returns the configured menu entries.
func (c *Config) GetMenu() []models.MenuEntry { return c.Menu }

// GetFooterMenu returns the configured footer menu entries.
func (c *Config) GetFooterMenu() []models.MenuEntry { return c.FooterMenu }

// GetAuthor returns the configured author metadata.
func (c *Config) GetAuthor() models.AuthorConfig { return c.Author }

// GetSocial returns the social card configuration.
func (c *Config) GetSocial() models.SocialCardsConfig { return c.SocialCards }

// GetFeatures returns the features configuration.
func (c *Config) GetFeatures() models.FeaturesConfig { return c.Features }

// GetSiteTitle returns the site title.
func (c *Config) GetSiteTitle() string { return c.Title }

// GetLogo returns the path to the site logo.
func (c *Config) GetLogo() string { return c.Logo }

// GetBaseURL returns the configured base URL.
func (c *Config) GetBaseURL() string { return c.BaseURL }

// GetContentPrefix returns the content prefix path.
func (c *Config) GetContentPrefix() string { return c.ContentPrefix }

// GetTemplateDir returns the directory containing theme templates.
func (c *Config) GetTemplateDir() string { return c.TemplateDir }

// GetStaticDir returns the directory containing global static assets.
func (c *Config) GetStaticDir() string { return c.StaticDir }

// GetLayoutsDir returns the directory for site-level template overrides.
func (c *Config) GetLayoutsDir() string { return c.LayoutsDir }

// GetContentDir returns the directory containing markdown content.
func (c *Config) GetContentDir() string { return c.ContentDir }

// IsDevMode returns whether the build is running in development mode.
func (c *Config) IsDevMode() bool { return c.IsDev }

// GetNavbar returns the navbar branding configuration.
func (c *Config) GetNavbar() models.NavbarIdentityConfig { return c.Navbar }

// GetHomeBadge returns the home page badge label.
func (c *Config) GetHomeBadge() string {
	if c.HomeBadge == "" {
		return "Latest Items"
	}
	return c.HomeBadge
}
