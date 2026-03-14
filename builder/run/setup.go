package run

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// VerifyTheme checks if the theme directories exist
func VerifyTheme(cfg *config.Config, logger *slog.Logger) {
	VerifyThemeFs(afero.NewOsFs(), cfg, logger)
}

// VerifyThemeFs checks if the theme directories exist using the provided filesystem
func VerifyThemeFs(fs afero.Fs, cfg *config.Config, logger *slog.Logger) {
	themePath := filepath.Join(cfg.ThemeDir, cfg.Theme)
	if exists, _ := afero.Exists(fs, themePath); !exists {
		logger.Error("Theme not found",
			"theme", cfg.Theme,
			"path", themePath,
			"hint", "Please ensure you have installed the theme into '"+cfg.ThemeDir+"/"+cfg.Theme+"/'")
		logger.Info("Theme installation:", "example", "git clone <theme-repo-url> "+filepath.Join(cfg.ThemeDir, cfg.Theme))
		if !utils.TestingMode {
			os.Exit(1)
		}
		return
	}

	templatePath := cfg.TemplateDir
	if exists, _ := afero.Exists(fs, templatePath); !exists {
		logger.Error("Theme templates directory not found",
			"theme", cfg.Theme,
			"path", templatePath,
			"hint", "Theme must have a 'templates' directory")
		if !utils.TestingMode {
			os.Exit(1)
		}
		return
	}

	staticPath := cfg.StaticDir
	if exists, _ := afero.Exists(fs, staticPath); !exists {
		logger.Warn("Theme static directory not found, creating empty",
			"theme", cfg.Theme,
			"path", staticPath)
		_ = fs.MkdirAll(staticPath, 0755)
	}
}

// SetupCacheDirectories creates required cache folders
func SetupCacheDirectories(cfg *config.Config, logger *slog.Logger) {
	SetupCacheDirectoriesFs(afero.NewOsFs(), cfg, logger)
}

// SetupCacheDirectoriesFs creates required cache folders using the provided filesystem
func SetupCacheDirectoriesFs(fs afero.Fs, cfg *config.Config, logger *slog.Logger) {
	if err := fs.MkdirAll(cfg.CacheDir, 0755); err != nil {
		logger.Error("Failed to create cache directory", "path", cfg.CacheDir, "error", err)
		if !utils.TestingMode {
			os.Exit(1)
		}
		return
	}
	if err := fs.MkdirAll(filepath.Join(cfg.CacheDir, "social-cards"), 0755); err != nil {
		logger.Error("Failed to create social-cards cache directory", "error", err)
	}
	if err := fs.MkdirAll(filepath.Join(cfg.CacheDir, "assets"), 0755); err != nil {
		logger.Error("Failed to create assets cache directory", "error", err)
	}
	if err := fs.MkdirAll(filepath.Join(cfg.CacheDir, "images"), 0755); err != nil {
		logger.Error("Failed to create images cache directory", "error", err)
	}
	if err := fs.MkdirAll(filepath.Join(cfg.CacheDir, "pwa-icons"), 0755); err != nil {
		logger.Error("Failed to create pwa-icons cache directory", "error", err)
	}
}

// SetupCacheManager opens and verifies the bolt DB cache
func SetupCacheManager(cfg *config.Config, logger *slog.Logger) (*cache.Manager, *cache.DiagramCacheAdapter) {
	cacheTimeout := cfg.Build.CacheDBTimeout
	cm, err := cache.OpenWithTimeout(cfg.CacheDir, cfg.IsDev, cacheTimeout)
	if err != nil {
		logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
		return nil, nil
	}

	if errors, verifyErr := cm.QuickVerify(); verifyErr != nil || len(errors) > 0 {
		logger.Warn("Cache integrity issues detected, forcing rebuild", "errors", len(errors))
		if len(errors) > 0 && len(errors) <= 5 {
			for _, e := range errors {
				logger.Debug("Cache error", "detail", e)
			}
		}
		cfg.ForceRebuild = true
		_ = cm.ClearAll()
	}

	cacheID := generateCacheID()
	needsRebuild, _ := cm.VerifyCacheID(cacheID)
	if needsRebuild {
		logger.Info("Cache fingerprint changed, triggering rebuild")
		cfg.ForceRebuild = true
		_ = cm.SetCacheID(cacheID)
	}

	diagramAdapter := cache.NewDiagramCacheAdapter(cm)
	return cm, diagramAdapter
}

func generateCacheID() string {
	components := []string{
		"kosh:1.0",
		"goldmark:1.7",
		"d2:0.7",
		"katex:embedded",
	}

	var sb strings.Builder
	sb.Grow(50)
	for _, c := range components {
		sb.WriteString(c)
		sb.WriteString("|")
	}

	return cache.HashString(sb.String())
}

// LoadThemeMetadata reads the metadata out of the theme yaml
func LoadThemeMetadata(cfg *config.Config, sourceFs afero.Fs, logger *slog.Logger) {
	themeMetadata := config.ThemeConfig{
		Name:               cfg.Theme,
		SupportsVersioning: false,
	}
	themePath := filepath.Join(cfg.ThemeDir, cfg.Theme)
	themeYamlPath := filepath.Join(themePath, "theme.yaml")
	if data, err := afero.ReadFile(sourceFs, themeYamlPath); err == nil {
		if err := yaml.Unmarshal(data, &themeMetadata); err != nil {
			logger.Warn("Failed to parse theme.yaml", "error", err)
		}
	}
	cfg.ThemeMetadata = themeMetadata
}
