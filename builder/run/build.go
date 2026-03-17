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
package run

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	"github.com/Kush-Singh-26/kosh/builder/utils/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// refreshBuildSession creates a fresh Transaction and Sink for a new build pass.
func (b *Builder) refreshBuildSession() {
	// If we already have a sink/tx (e.g. injected in tests), don't overwrite it
	if b.Sink == nil || !utils.TestingMode {
		useStaging := !b.cfg.IsDev || b.state.isCleanBuild
		// Explicit cleanup before creating new transaction for clean builds
		if useStaging {
			tx.CleanupStaleBuildDirs(b.cfg.OutputDir)
		}
		b.Tx = tx.NewBuildTransaction(b.cfg.OutputDir, useStaging)
		b.Sink = fspkg.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)
	}

	// Consolidated service reconfiguration - single explicit call per service
	b.deps.Post.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.deps.Asset.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.deps.Render.ReconfigureForBuild(b.Sink, b.SourceFs)
}

// build executes the build logic without locking (internal use)
func (b *Builder) build(ctx context.Context) error {
	setup, err := b.setupPhase(ctx)
	if err != nil {
		return err
	}

	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	assets := b.assetPhase(ctx, contentAssetsChan)
	scan := b.scanPhase(ctx, contentAssetsChan)

	if err := b.processPhase(ctx, setup, assets, scan); err != nil {
		return err
	}

	return b.finalizePhase(ctx, setup.wasmWg)
}

func (b *Builder) buildAssetOnly(ctx context.Context) error {
	if b.metrics != nil {
		b.metrics.Reset()
	}
	b.deps.Render.SetAssets(map[string]string{})

	slog.Info("Building assets...")
	assetTimer := timeutil.StartPhase("Asset building")

	// Start fresh session/tracking state
	b.refreshBuildSession()

	assets, err := b.deps.Asset.BuildForAssetChange(ctx)
	assetTimer.Stop()
	if err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}

	b.deps.Render.SetAssets(assets)
	b.deps.Render.ClearRenderedFiles()
	b.deps.Render.SetAssetsGate(nil)
	b.deps.Post.SetAssetsGate(nil)
	b.state.forceGenerators.Store(true)

	fileChan := make(chan models.ScannedFile, 1024)
	go func() {
		defer close(fileChan)
		if _, err := b.deps.Scanner.Scan(ctx, b.cfg.ContentDir, b.SourceFs, b.cfg, fileChan); err != nil {
			slog.Debug("metadata scan in asset-only build error", "error", err)
		}
	}()

	shouldForce := false
	forceSocialRebuild := false
	outputMissing := false
	_, err = b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing, fileChan)
	if err != nil {
		return fmt.Errorf("post processing failed: %w", err)
	}

	if err := b.Tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}

	b.CleanupOrphans()

	b.metrics.RecordEnd()
	DevLogSuccess("Build complete")
	b.metrics.Print()

	return nil
}

func (b *Builder) Build(ctx context.Context) error {
	// Prevent concurrent builds
	b.state.buildMu.Lock()
	defer b.state.buildMu.Unlock()

	// Reset per-build metrics so watch-mode rebuilds don't accumulate counters.
	if b.metrics != nil {
		b.metrics.Reset()
	}

	// Acquire build lock to prevent concurrent builds (skip in tests)
	var buildLock *fspkg.FileLock
	var lockErr error
	if !utils.TestingMode {
		buildLock, lockErr = fspkg.AcquireBuildLock(b.cfg.OutputDir)
		if lockErr != nil {
			if !b.cfg.ForceLock {
				return fmt.Errorf("could not acquire build lock: %w (use --force-lock to override)", lockErr)
			}
			b.logger.Warn("Acquiring build lock failed, but continuing due to --force-lock", "error", lockErr)
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

func (b *Builder) copyStaticAndBuildAssets(ctx context.Context) error {
	if err := b.deps.Asset.Build(ctx); err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}
	return nil
}
