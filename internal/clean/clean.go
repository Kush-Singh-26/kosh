// Package clean handles the removal of build artifacts.
package clean

import (
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
)

// Run removes the build output directory and potentially the cache.
func Run(args []string, cleanCache bool) error {
	fs := afero.NewOsFs()
	cfg := config.Load(args)

	// Remove output directory
	if cfg.OutputDir != "" {
		slog.Info("Cleaning output directory", "path", cfg.OutputDir)
		if err := fs.RemoveAll(cfg.OutputDir); err != nil {
			slog.Warn("Failed to remove output directory", "error", err)
		}
	}

	// Remove cache directory if requested
	if cleanCache && cfg.CacheDir != "" {
		slog.Info("Cleaning cache directory", "path", cfg.CacheDir)
		if err := fs.RemoveAll(cfg.CacheDir); err != nil {
			slog.Warn("Failed to remove cache directory", "error", err)
		}
	}

	return nil
}

// RunFs is a testing entry point that allows providing a custom filesystem.
func RunFs(fs afero.Fs, args []string, cleanCache bool) error {
	cfg := config.Load(args)

	if cfg.OutputDir != "" {
		_ = fs.RemoveAll(cfg.OutputDir)
	}

	if cleanCache && cfg.CacheDir != "" {
		_ = fs.RemoveAll(cfg.CacheDir)
	}

	return nil
}
