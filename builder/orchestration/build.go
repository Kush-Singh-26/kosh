// Package orchestration orchestrates full and incremental site builds.
//
// Build orchestration call chain:
//
//	Build() → processPosts() → runParsePhase() → parseWorkerTask() → PostService.Process()
//
// This 4-level chain is intentional: Build() coordinates high-level phases,
// processPosts() handles post-specific logic, runParsePhase() manages worker pools,
// and parseWorkerTask() executes individual parses. The separation enables
// parallelism, progress tracking, and error isolation.
package orchestration

import (
	"context"
	"fmt"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/async"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// refreshBuildSession creates a fresh Transaction and Sink for a new build pass.
func (b *Engine) refreshBuildSession() {
	// Clear per-file content cache so dev rebuilds don't serve stale data
	async.ClearSyncCache()
	// If we already have a sink/tx (e.g. injected in tests), don't overwrite it
	if b.Sink == nil || !b.Ctx.IsTesting {
		useStaging := !b.Cfg.IsDev || b.State.IsCleanBuild
		// Explicit cleanup before creating new transaction for clean builds
		if useStaging {
			tx.CleanupStaleBuildDirs(b.Cfg.OutputDir)
		}
		b.Tx = tx.NewBuildTransaction(b.Cfg.OutputDir, useStaging)
		b.SetSink(fspkg.NewDiskSink(b.Tx.StagingDir(), b.Cfg.OutputDir))
	} else {
		// Even if sink is already set (e.g. in tests), reconfigure services with current Fs
		b.SetSink(b.Sink)
	}
}

// BuildLocked executes the build logic without locking.
// This is used internally and by the incremental manager when it already holds the build lock.
func (b *Engine) BuildLocked(ctx context.Context) error {
	setup, err := b.setupPhase(ctx)
	if err != nil {
		return err
	}

	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	assets := b.assetPhase(ctx, contentAssetsChan)
	scan := b.scanPhase(ctx, contentAssetsChan)

	if err := b.processPhase(ctx, setup, assets, scan); err != nil {
		close(contentAssetsChan)
		return err
	}

	close(contentAssetsChan)
	return b.finalizePhase(ctx, setup.wasmWg, assets.assetsReady)
}

func (b *Engine) buildAssetOnly(ctx context.Context) error {
	b.State.IsAssetOnlyBuild = true
	defer func() { b.State.IsAssetOnlyBuild = false }()

	// Start fresh session/tracking state
	b.refreshBuildSession()

	return b.Assets.BuildAssetOnly(ctx, func(ctx context.Context) error {
		b.Deps.Post.SetAssetsGate(nil)
		b.State.ForceGenerators.Store(true)

		metadataResult, err := b.Deps.Scanner.Scan(ctx, b.Cfg.ContentDir, b.Deps.SourceFs, b.Cfg, nil)
		if err != nil {
			return fmt.Errorf("metadata scan failed: %w", err)
		}

		shouldForce := false
		forceSocialRebuild := false
		outputMissing := false
		_, err = b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing, metadataResult.Files)
		if err != nil {
			return fmt.Errorf("post processing failed: %w", err)
		}

		// Remove original raster images when .webp equivalents exist
		assets.CleanupOriginalImages(b.Tx.StagingDir())

		if err := b.Tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to publish build transaction: %w", err)
		}

		b.cleanupOrphans()

		b.Deps.Metrics.RecordEnd()
		b.Deps.Logger.Info("Build complete")
		b.Deps.Metrics.Print()

		return nil
	})
}

func (b *Engine) Build(ctx context.Context) error {
	// Prevent concurrent builds
	b.State.BuildMu.Lock()
	defer b.State.BuildMu.Unlock()

	// Reset per-build metrics so watch-mode rebuilds don't accumulate counters.
	if b.Deps.Metrics != nil {
		b.Deps.Metrics.Reset()
	}

	// Acquire build lock to prevent concurrent builds (skip in tests)
	var buildLock *fspkg.FileLock
	var lockErr error
	if !b.Ctx.IsTesting {
		buildLock, lockErr = fspkg.AcquireBuildLock(b.Cfg.OutputDir)
		if lockErr != nil {
			if !b.Cfg.ForceLock {
				return fmt.Errorf("could not acquire build lock: %w (use --force-lock to override)", lockErr)
			}
			b.Deps.Logger.Warn("Acquiring build lock failed, but continuing due to --force-lock", "error", lockErr)
		} else {
			defer func() {
				if buildLock != nil {
					if err := buildLock.Release(); err != nil {
						b.Deps.Logger.Error("Failed to release build lock", "error", err)
					}
				}
			}()
		}
	}

	return b.BuildLocked(ctx)
}
