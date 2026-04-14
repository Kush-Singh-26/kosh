//go:build !wasm

package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

const (
	percentScale = 100
)

// finalizeBuild writes post-build files and commits the transaction.
func (engineInstance *Engine) finalizeBuild(ctx context.Context, wasmWaitGroup *sync.WaitGroup, assetsReadySignal <-chan struct{}) error {
	// Write .nojekyll file
	if err := engineInstance.artifactSink.WriteFile(filepath.Join(engineInstance.Cfg.OutputDir, ".nojekyll"), []byte{}); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}
	engineInstance.Deps.Render.RegisterFile(filepath.Join(engineInstance.Cfg.OutputDir, ".nojekyll"))

	// Sync/Commit transaction
	engineInstance.Deps.Logger.Info("Publishing output...")
	syncTimer := timeutil.StartPhase("Publish")
	// Ensure WASM compilation and PWA generation finished before deploying and publishing
	wasmWaitGroup.Wait()

	// Reset ShouldForceRebuild AFTER all async checks have completed
	engineInstance.Cfg.ShouldForceRebuild = false

	if err := engineInstance.Deps.Wasm.Deploy(ctx, engineInstance.artifactSink); err != nil {
		engineInstance.Deps.Logger.Warn("Failed to deploy Search WASM", "error", err)
	}

	// Ensure asset pipeline finished so converted-image map is complete.
	if assetsReadySignal != nil {
		select {
		case <-assetsReadySignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhaseAssets, 0)
	}

	// Remove original raster images (.png/.jpg/.jpeg) when .webp equivalents exist.
	// This ensures the published output contains only WebP images (except critical assets).
	assets.CleanupOriginalImages(engineInstance.buildTransaction.StagingDir())

	if err := engineInstance.buildTransaction.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
}

// finalizePhase handles post-build cleanup and commit.
func (engineInstance *Engine) finalizePhase(ctx context.Context, wasmWaitGroup *sync.WaitGroup, assetsReadySignal <-chan struct{}) error {
	// Post-build files and commit
	if err := engineInstance.finalizeBuild(ctx, wasmWaitGroup, assetsReadySignal); err != nil {
		return err
	}

	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhasePublish, 0)
	}

	// Cleanup orphans (Dev mode only)
	engineInstance.cleanupOrphans()

	// Clear memory state
	engineInstance.Deps.Render.ClearRenderedFiles()

	// Build complete. Log summary insights.
	engineInstance.Deps.Metrics.RecordEnd()
	engineInstance.printBuildInsights()
	if engineInstance.Health != nil {
		engineInstance.Health.LogSummary()
	}

	if engineInstance.Deps.Reporter != nil {
		metricsInstance := engineInstance.Deps.Metrics
		hits := metricsInstance.CacheHits.Load()
		misses := metricsInstance.CacheMisses.Load()
		total := hits + misses
		hitRate := float64(0)
		if total > 0 {
			hitRate = float64(hits) / float64(total)
		}

		engineInstance.Deps.Reporter.Finish(ui.BuildStats{
			Duration:   metricsInstance.TotalDuration(),
			HitRate:    hitRate,
			Posts:      int(metricsInstance.PostsProcessed.Load()),
			Assets:     int(metricsInstance.AssetsProcessed.Load()),
			Optimized:  int(metricsInstance.ImagesOptimized.Load()),
			SavedBytes: metricsInstance.OriginalImageSize.Load() - metricsInstance.OptimizedImageSize.Load(),
		})
	} else {
		engineInstance.Deps.Logger.Info("Build complete")
	}

	return nil
}

func (engineInstance *Engine) printBuildInsights() {
	metricsInstance := engineInstance.Deps.Metrics
	if metricsInstance == nil {
		return
	}

	hits := metricsInstance.CacheHits.Load()
	misses := metricsInstance.CacheMisses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * percentScale
	}

	originalSize := metricsInstance.OriginalImageSize.Load()
	optimizedSize := metricsInstance.OptimizedImageSize.Load()
	saved := int64(0)
	if originalSize > optimizedSize {
		saved = originalSize - optimizedSize
	}
	saveRate := float64(0)
	if originalSize > 0 {
		saveRate = float64(saved) / float64(originalSize) * percentScale
	}

	engineInstance.Deps.Logger.Info("Build Insights",
		"posts", metricsInstance.PostsProcessed.Load(),
		"cache_hit_rate", fmt.Sprintf("%.1f%%", hitRate),
		"images_optimized", metricsInstance.ImagesOptimized.Load(),
		"image_savings", fmt.Sprintf("%.1f%% (%s)", saveRate, formatBytes(saved)),
	)
}

func formatBytes(bytes int64) string {
	const unit = bytesPerKiB
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
