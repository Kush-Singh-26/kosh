package run

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

func TestVerifyThemeFs_Detached(t *testing.T) {
	fs := afero.NewMemMapFs()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

	// Create a theme in a completely different "drive" or path
	externalThemeDir := "/external/themes"
	themeName := "my-detached-theme"
	themePath := filepath.Join(externalThemeDir, themeName)

	_ = fs.MkdirAll(filepath.Join(themePath, "templates"), 0755)
	_ = afero.WriteFile(fs, filepath.Join(themePath, "templates", "layout.html"), []byte("<html></html>"), 0644)

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			Theme:       themeName,
			ThemeDir:    externalThemeDir,
			TemplateDir: filepath.Join(themePath, "templates"),
			StaticDir:   filepath.Join(themePath, "static"),
		},
	}

	// This should pass without error/exit
	VerifyThemeFs(fs, cfg, logger)

	// Verify static dir was created (it didn't exist)
	exists, _ := afero.DirExists(fs, cfg.StaticDir)
	if !exists {
		t.Errorf("Expected static directory to be created at %s", cfg.StaticDir)
	}
}

func TestVerifyThemeFs_Invalid(t *testing.T) {
	fs := afero.NewMemMapFs()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			Theme:    "non-existent",
			ThemeDir: "themes",
		},
	}

	// VerifyThemeFs should log error and return (because TestingMode=true)
	VerifyThemeFs(fs, cfg, logger)

	// If it didn't crash/exit, it passed the test of graceful failure in testing mode.
}
