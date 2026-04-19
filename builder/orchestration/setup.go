package orchestration

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	cachepkg "github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/config"
	pathFs "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)


const (
	cacheDirMode       = 0755
	cacheErrorLogLimit = 5
	cacheIDBufferSize  = 50
)

// VerifyTheme checks if the theme directories exist
func VerifyTheme(configInstance *config.Config, logger *slog.Logger) {
	VerifyThemeFs(afero.NewOsFs(), configInstance, logger, pathFs.DetectTestingMode())
}

// VerifyThemeFs checks if the theme directories exist using the provided filesystem
func VerifyThemeFs(sourceFs afero.Fs, configInstance *config.Config, logger *slog.Logger, isTesting bool) {
	themePath := filepath.Join(configInstance.ThemeDir, configInstance.Theme)
	if exists, err := afero.Exists(sourceFs, themePath); err != nil {
		logger.Error("Failed to check theme directory", "path", themePath, "error", err)
		if !isTesting {
			os.Exit(1)
		}
		return
	} else if !exists {
		logger.Error("Theme not found",
			"theme", configInstance.Theme,
			"path", themePath,
			"hint", "Please ensure you have installed the theme into '"+configInstance.ThemeDir+"/"+configInstance.Theme+"/'")
		logger.Info("Theme installation:", "example", "git clone <theme-repo-url> "+filepath.Join(configInstance.ThemeDir, configInstance.Theme))
		if !isTesting {
			os.Exit(1)
		}
		return
	}

	templatePath := configInstance.TemplateDir
	if exists, err := afero.Exists(sourceFs, templatePath); err != nil {
		logger.Error("Failed to check theme templates directory", "path", templatePath, "error", err)
		if !isTesting {
			os.Exit(1)
		}
		return
	} else if !exists {
		logger.Error("Theme templates directory not found",
			"theme", configInstance.Theme,
			"path", templatePath,
			"hint", "Theme must have a 'templates' directory")
		if !isTesting {
			os.Exit(1)
		}
		return
	}

	staticPath := configInstance.StaticDir
	if exists, err := afero.Exists(sourceFs, staticPath); err != nil {
		logger.Warn("Failed to check theme static directory", "path", staticPath, "error", err)
	} else if !exists {
		logger.Warn("Theme static directory not found, creating empty",
			"theme", configInstance.Theme,
			"path", staticPath)
		if err := sourceFs.MkdirAll(staticPath, cacheDirMode); err != nil {
			logger.Warn("Failed to create empty static directory", "path", staticPath, "error", err)
		}
	}
}

// SetupCacheDirectories creates required cache folders
func SetupCacheDirectories(configInstance *config.Config, logger *slog.Logger) {
	SetupCacheDirectoriesFs(afero.NewOsFs(), configInstance, logger, pathFs.DetectTestingMode())
}

// SetupCacheDirectoriesFs creates required cache folders using the provided filesystem
func SetupCacheDirectoriesFs(sourceFs afero.Fs, configInstance *config.Config, logger *slog.Logger, isTesting bool) {
	if err := sourceFs.MkdirAll(configInstance.CacheDir, cacheDirMode); err != nil {
		logger.Error("Failed to create cache directory", "path", configInstance.CacheDir, "error", err)
		if !isTesting {
			os.Exit(1)
		}
		return
	}
	if err := sourceFs.MkdirAll(filepath.Join(configInstance.CacheDir, "social-cards"), cacheDirMode); err != nil {
		logger.Error("Failed to create social-cards cache directory", "error", err)
	}
	if err := sourceFs.MkdirAll(filepath.Join(configInstance.CacheDir, "assets"), cacheDirMode); err != nil {
		logger.Error("Failed to create assets cache directory", "error", err)
	}
	if err := sourceFs.MkdirAll(filepath.Join(configInstance.CacheDir, "images"), cacheDirMode); err != nil {
		logger.Error("Failed to create images cache directory", "error", err)
	}
	if err := sourceFs.MkdirAll(filepath.Join(configInstance.CacheDir, "pwa-icons"), cacheDirMode); err != nil {
		logger.Error("Failed to create pwa-icons cache directory", "error", err)
	}
}

// SetupCacheManager opens and verifies the bolt DB cache
func SetupCacheManager(configInstance *config.Config, logger *slog.Logger) (*cachepkg.Manager, *cachepkg.DiagramCacheAdapter, error) {
	cacheTimeout := configInstance.Build.CacheDBTimeout
	cacheManager, err := cachepkg.OpenWithTimeout(configInstance.CacheDir, configInstance.IsDev, cacheTimeout)
	if err != nil {
		logger.Warn("Failed to open cache database", "error", err)
		return nil, nil, err
	}

	if cacheErrors, verifyErr := cacheManager.QuickVerify(); verifyErr != nil || len(cacheErrors) > 0 {
		logger.Warn("Cache integrity issues detected, forcing rebuild", "errors", len(cacheErrors))
		if verifyErr != nil {
			logger.Debug("Verification failed", "error", verifyErr)
		}
		if len(cacheErrors) > 0 && len(cacheErrors) <= cacheErrorLogLimit {
			for _, detail := range cacheErrors {
				logger.Debug("Cache error", "detail", detail)
			}
		}
		configInstance.ShouldForceRebuild = true
		if err := cacheManager.ClearAll(); err != nil {
			logger.Warn("Failed to clear cache", "error", err)
		}
	}

	cacheID := generateCacheID()
	needsRebuild, err := cacheManager.VerifyCacheID(cacheID)
	if err != nil {
		logger.Warn("Failed to verify cache ID", "error", err)
		needsRebuild = true // Force rebuild on error
	}

	if needsRebuild {
		logger.Info("Cache fingerprint changed, triggering rebuild")
		configInstance.ShouldForceRebuild = true
		if err := cacheManager.SetCacheID(cacheID); err != nil {
			logger.Warn("Failed to set cache ID", "error", err)
		}
	}

	diagramAdapter := cachepkg.NewDiagramCacheAdapter(cacheManager)
	return cacheManager, diagramAdapter, nil
}

func generateCacheID() string {
	components := []string{
		"kosh:1.0",
		"goldmark:1.7",
		"d2:0.7",
		"katex:embedded",
	}

	var cacheIDBuilder strings.Builder
	cacheIDBuilder.Grow(cacheIDBufferSize)
	for _, component := range components {
		cacheIDBuilder.WriteString(component)
		cacheIDBuilder.WriteString("|")
	}

	return core.HashString(cacheIDBuilder.String())
}

// LoadThemeMetadata reads the metadata out of the theme yaml
func LoadThemeMetadata(configInstance *config.Config, sourceFs afero.Fs, logger *slog.Logger) {
	themeMetadata := config.ThemeConfig{
		Name: configInstance.Theme,
	}
	themePath := filepath.Join(configInstance.ThemeDir, configInstance.Theme)
	themeYamlPath := filepath.Join(themePath, "theme.yaml")
	if data, readError := afero.ReadFile(sourceFs, themeYamlPath); readError == nil {
		if unmarshalError := yaml.Unmarshal(data, &themeMetadata); unmarshalError != nil {
			logger.Warn("Failed to parse theme.yaml", "error", unmarshalError)
		}
	}
	configInstance.ThemeMetadata = themeMetadata
}
