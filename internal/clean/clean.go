// Package clean handles the removal of build artifacts.
package clean

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/retry"
	"github.com/spf13/afero"
)

const (
	cleanupRetryMax   = 5
	cleanupRetryDelay = 10 * time.Millisecond
)

var (
	errOutputRemoval = errors.New("failed to remove output directory")
	errCacheRemoval  = errors.New("failed to remove cache directory")
)

// Run removes the build output directory and potentially the cache.
func Run(ctx context.Context, args []string, cleanCache bool) error {
	cfg := config.Load(args)
	return RunWithConfig(ctx, cfg, cleanCache)
}

// RunWithConfig removes the build output directory and potentially the cache using the provided config.
// This allows callers to reuse the same config instance loaded once for banner and cleanup.
func RunWithConfig(ctx context.Context, cfg *config.Config, cleanCache bool) error {
	var cleanupErrors []error

	// Remove output directory
	if cfg.OutputDir != "" {
		slog.Info("Cleaning output directory", "path", cfg.OutputDir)
		if err := retry.RemoveAllWithRetry(ctx, cfg.OutputDir, cleanupRetryMax, cleanupRetryDelay); err != nil {
			slog.Error("Failed to remove output directory", "path", cfg.OutputDir, "error", err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errOutputRemoval, err))
		}
	}

	// Remove cache directory if requested
	if cleanCache && cfg.CacheDir != "" {
		slog.Info("Cleaning cache directory", "path", cfg.CacheDir)
		if err := retry.RemoveAllWithRetry(ctx, cfg.CacheDir, cleanupRetryMax, cleanupRetryDelay); err != nil {
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
func RunFs(ctx context.Context, fs afero.Fs, args []string, cleanCache bool) error {
	cfg := config.Load(args)
	return RunFsWithConfig(ctx, fs, cfg, cleanCache)
}

// RunFsWithConfig is a testing entry point that allows providing a custom filesystem and config.
func RunFsWithConfig(ctx context.Context, _ afero.Fs, cfg *config.Config, cleanCache bool) error {
	var cleanupErrors []error

	if cfg.OutputDir != "" {
		if err := retry.RemoveAllWithRetry(ctx, cfg.OutputDir, cleanupRetryMax, cleanupRetryDelay); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errOutputRemoval, err))
		}
	}

	if cleanCache && cfg.CacheDir != "" {
		if err := retry.RemoveAllWithRetry(ctx, cfg.CacheDir, cleanupRetryMax, cleanupRetryDelay); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%w: %w", errCacheRemoval, err))
		}
	}

	if len(cleanupErrors) > 0 {
		return errors.Join(cleanupErrors...)
	}
	return nil
}
