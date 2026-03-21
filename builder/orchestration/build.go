// Package run orchestrates full and incremental site builds.
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

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// refreshBuildSession creates a fresh Transaction and Sink for a new build pass.
func (b *Engine) refreshBuildSession() {
	// If we already have a sink/tx (e.g. injected in tests), don't overwrite it
	if b.Sink == nil || !b.Ctx.IsTesting {
		useStaging := !b.Cfg.IsDev || b.State.IsCleanBuild
		// Explicit cleanup before creating new transaction for clean builds
		if useStaging {
			tx.CleanupStaleBuildDirs(b.Cfg.OutputDir)
		}
		b.Tx = tx.NewBuildTransaction(b.Cfg.OutputDir, useStaging)
		b.Sink = fspkg.NewDiskSink(b.Tx.StagingDir(), b.Cfg.OutputDir)
	}

	// Consolidated service reconfiguration - single explicit call per service
	b.Deps.Post.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.Deps.Asset.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.Deps.Render.ReconfigureForBuild(b.Sink, b.SourceFs)
}

// build executes the build logic without locking (internal use)
func (b *Engine) build(ctx context.Context) error {
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
	return b.finalizePhase(ctx, setup.wasmWg)
}

func (b *Engine) buildAssetOnly(ctx context.Context) error {
	// Start fresh session/tracking state
	b.refreshBuildSession()

	return b.Assets.BuildAssetOnly(ctx, func(ctx context.Context) error {
		b.Deps.Post.SetAssetsGate(nil)
		b.State.ForceGenerators.Store(true)

		fileChan := make(chan models.ScannedFile, 1024)
		go func() {
			defer close(fileChan)
			if _, err := b.Deps.Scanner.Scan(ctx, b.Cfg.ContentDir, b.SourceFs, b.Cfg, fileChan); err != nil {
				b.Logger.Debug("metadata scan in asset-only build error", "error", err)
			}
		}()

		shouldForce := false
		forceSocialRebuild := false
		outputMissing := false
		_, err := b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing, fileChan)
		if err != nil {
			return fmt.Errorf("post processing failed: %w", err)
		}

		if err := b.Tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to publish build transaction: %w", err)
		}

		b.cleanupOrphans()

		b.Metrics.RecordEnd()
		b.Logger.Info("Build complete")
		b.Metrics.Print()

		return nil
	})
}

func (b *Engine) Build(ctx context.Context) error {
	// Prevent concurrent builds
	b.State.BuildMu.Lock()
	defer b.State.BuildMu.Unlock()

	// Reset per-build metrics so watch-mode rebuilds don't accumulate counters.
	if b.Metrics != nil {
		b.Metrics.Reset()
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
			b.Logger.Warn("Acquiring build lock failed, but continuing due to --force-lock", "error", lockErr)
		} else {
			defer func() {
				if buildLock != nil {
					_ = buildLock.Release()
				}
			}()
		}
	}

	return b.build(ctx)
}
