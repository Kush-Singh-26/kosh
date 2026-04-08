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

// finalizeBuild writes post-build files and commits the transaction.
func (b *Engine) finalizeBuild(ctx context.Context, wasmWg *sync.WaitGroup, assetsReady <-chan struct{}) error {
	// Write .nojekyll file
	if err := b.Sink.WriteFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"), []byte{}); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}
	b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"))

	// Sync/Commit transaction
	b.Deps.Logger.Info("Publishing output...")
	syncTimer := timeutil.StartPhase("Publish")
	// Ensure WASM compilation and PWA generation finished before deploying and publishing
	wasmWg.Wait()

	// Reset ForceRebuild AFTER all async checks have completed
	b.Cfg.ForceRebuild = false

	if err := b.Deps.Wasm.Deploy(ctx, b.Sink); err != nil {
		b.Deps.Logger.Warn("Failed to deploy Search WASM", "error", err)
	}

	// Ensure asset pipeline finished so converted-image map is complete.
	if assetsReady != nil {
		select {
		case <-assetsReady:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhaseAssets, 0)
	}

	// Remove original raster images (.png/.jpg/.jpeg) when .webp equivalents exist.
	// This ensures the published output contains only WebP images (except critical assets).
	assets.CleanupOriginalImages(b.Tx.StagingDir())

	if err := b.Tx.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
}

// finalizePhase handles post-build cleanup and commit.
func (b *Engine) finalizePhase(ctx context.Context, wasmWg *sync.WaitGroup, assetsReady <-chan struct{}) error {
	// Post-build files and commit
	if err := b.finalizeBuild(ctx, wasmWg, assetsReady); err != nil {
		return err
	}

	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhasePublish, 0)
	}

	// Cleanup orphans (Dev mode only)
	b.cleanupOrphans()

	// Clear memory state
	b.Deps.Render.ClearRenderedFiles()

	// Build complete. Log summary insights.
	b.Deps.Metrics.RecordEnd()
	b.printBuildInsights()
	if b.Health != nil {
		b.Health.LogSummary()
	}

	if b.Deps.Reporter != nil {
		m := b.Deps.Metrics
		hits := m.CacheHits.Load()
		misses := m.CacheMisses.Load()
		total := hits + misses
		hitRate := float64(0)
		if total > 0 {
			hitRate = float64(hits) / float64(total)
		}

		b.Deps.Reporter.Finish(ui.BuildStats{
			Duration:   m.TotalDuration(),
			HitRate:    hitRate,
			Posts:      int(m.PostsProcessed.Load()),
			Assets:     int(m.AssetsProcessed.Load()),
			Optimized:  int(m.ImagesOptimized.Load()),
			SavedBytes: m.OriginalImageSize.Load() - m.OptimizedImageSize.Load(),
		})
	} else {
		b.Deps.Logger.Info("Build complete")
	}

	return nil
}

func (b *Engine) printBuildInsights() {
	m := b.Deps.Metrics
	if m == nil {
		return
	}

	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	origSize := m.OriginalImageSize.Load()
	optSize := m.OptimizedImageSize.Load()
	saved := int64(0)
	if origSize > optSize {
		saved = origSize - optSize
	}
	saveRate := float64(0)
	if origSize > 0 {
		saveRate = float64(saved) / float64(origSize) * 100
	}

	b.Deps.Logger.Info("Build Insights",
		"posts", m.PostsProcessed.Load(),
		"cache_hit_rate", fmt.Sprintf("%.1f%%", hitRate),
		"images_optimized", m.ImagesOptimized.Load(),
		"image_savings", fmt.Sprintf("%.1f%% (%s)", saveRate, formatBytes(saved)),
	)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
