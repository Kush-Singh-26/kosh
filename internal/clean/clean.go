// Package clean handles the removal of build artifacts.
package clean

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
)

var (
	errOutputRemoval = errors.New("failed to remove output directory")
	errCacheRemoval  = errors.New("failed to remove cache directory")
)

// Run removes the build output directory and potentially the cache.
func Run(args []string, cleanCache bool) error {
	cfg := config.Load(args)
	return RunWithConfig(cfg, cleanCache)
}

// RunWithConfig removes the build output directory and potentially the cache using the provided config.
// This allows callers to reuse the same config instance loaded once for banner and cleanup.
func RunWithConfig(cfg *config.Config, cleanCache bool) error {
	fs := afero.NewOsFs()
	var cleanupErrors []error

	// Remove output directory
	if cfg.OutputDir != "" {
		slog.Info("Cleaning output directory", "path", cfg.OutputDir)
		if err := fs.RemoveAll(cfg.OutputDir); err != nil {
			slog.Error("Failed to remove output directory", "path", cfg.OutputDir, "error", err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errOutputRemoval, err))
		}
	}

	// Remove cache directory if requested
	if cleanCache && cfg.CacheDir != "" {
		slog.Info("Cleaning cache directory", "path", cfg.CacheDir)
		if err := fs.RemoveAll(cfg.CacheDir); err != nil {
			slog.Error("Failed to remove cache directory", "path", cfg.CacheDir, "error", err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errCacheRemoval, err))
		}
	}

	if len(cleanupErrors) > 0 {
		return errors.Join(cleanupErrors...)
	}
	return nil
}

// RunFs is a testing entry point that allows providing a custom filesystem.
func RunFs(fs afero.Fs, args []string, cleanCache bool) error {
	cfg := config.Load(args)
	return RunFsWithConfig(fs, cfg, cleanCache)
}

// RunFsWithConfig is a testing entry point that allows providing a custom filesystem and config.
func RunFsWithConfig(fs afero.Fs, cfg *config.Config, cleanCache bool) error {
	var cleanupErrors []error

	if cfg.OutputDir != "" {
		if err := fs.RemoveAll(cfg.OutputDir); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errOutputRemoval, err))
		}
	}

	if cleanCache && cfg.CacheDir != "" {
		if err := fs.RemoveAll(cfg.CacheDir); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errCacheRemoval, err))
		}
	}

	if len(cleanupErrors) > 0 {
		return errors.Join(cleanupErrors...)
	}
	return nil
}
