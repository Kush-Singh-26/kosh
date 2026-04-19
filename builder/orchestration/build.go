// Package orchestration orchestrates full and incremental site builds.
//
// Build orchestration call chain:
//
//	Build() → processPosts() → runParsePhase() → parseWorkerTask() → PostService.Process()
//
// This 4-level chain is intentional: Build() coordinates high-level phases,
// processPosts() handles Content-specific logic, runParsePhase() manages worker pools,
// and parseWorkerTask() executes individual parses. The separation enables
// parallelism, progress tracking, and error isolation.
package orchestration

import (
	"context"
	"fmt"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
)

// refreshBuildSession creates a fresh Transaction and Sink for a new build pass.
func (engineInstance *Engine) refreshBuildSession(ctx context.Context) {
	// Clear per-file content cache so dev rebuilds don't serve stale data
	fspkg.ClearSyncCache()
	// If we already have a sink/tx (e.g. injected in tests), don't overwrite it
	if engineInstance.artifactSink == nil || !engineInstance.Ctx.IsTesting {
		useStaging := (!engineInstance.Cfg.IsDev || engineInstance.State.IsCleanBuild) && !engineInstance.Cfg.NoStaging
		// Explicit cleanup before creating new transaction for clean builds
		if useStaging {
			tx.CleanupStaleBuildDirs(ctx, engineInstance.Cfg.OutputDir)
		}
		engineInstance.buildTransaction = tx.NewBuildTransaction(ctx, engineInstance.Cfg.OutputDir, useStaging)
		engineInstance.SetSink(fspkg.NewDiskSink(engineInstance.buildTransaction.StagingDir(), engineInstance.Cfg.OutputDir))
	} else {
		// Even if sink is already set (e.g. in tests), reconfigure services with current Fs
		engineInstance.SetSink(engineInstance.artifactSink)
	}
}

// BuildLocked executes the build logic without locking.
// This is used internally and by the incremental manager when it already holds the build lock.
func (engineInstance *Engine) BuildLocked(ctx context.Context) error {
	setup, setupError := engineInstance.setupPhase(ctx)
	if setupError != nil {
		return setupError
	}

	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	assetsResult := engineInstance.assetPhase(ctx, contentAssetsChan)
	scanResult := engineInstance.scanPhase(ctx, contentAssetsChan)

	if processError := engineInstance.processPhase(ctx, setup, assetsResult, scanResult); processError != nil {
		close(contentAssetsChan)
		return processError
	}

	close(contentAssetsChan)
	return engineInstance.finalizePhase(ctx, setup.wasmWg, assetsResult.assetsReadySignal)
}

func (engineInstance *Engine) buildAssetOnly(ctx context.Context) error {
	engineInstance.State.IsAssetOnlyBuild = true
	defer func() { engineInstance.State.IsAssetOnlyBuild = false }()

	// Start fresh session/tracking state
	engineInstance.refreshBuildSession(ctx)

	return engineInstance.Assets.BuildAssetOnly(ctx, func(ctx context.Context) error {
		engineInstance.Deps.Content.SetAssetsGate(nil)
		engineInstance.State.ForceGenerators.Store(true)

		metadataResult, scanError := engineInstance.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: engineInstance.Cfg.ContentDir,
			SrcFs:      engineInstance.Deps.SourceFs,
			Cfg:        engineInstance.Cfg,
			FileChan:   nil,
		})
		if scanError != nil {
			return fmt.Errorf("metadata scan failed: %w", scanError)
		}

		// For asset-only builds, we FORCE Content re-rendering to update asset hashes in HTML.
		shouldForce := true
		forceSocialRebuild := false
		outputMissing := false
		_, processError := engineInstance.processPosts(ProcessPostsOptions{
			Ctx:                ctx,
			ShouldForce:        shouldForce,
			ForceSocialRebuild: forceSocialRebuild,
			OutputMissing:      outputMissing,
			Files:              metadataResult.Files,
		})
		if processError != nil {
			return fmt.Errorf("content processing failed: %w", processError)
		}

		// Remove original raster images when .webp equivalents exist
		assets.CleanupOriginalImages(ctx, engineInstance.buildTransaction.StagingDir())

		if commitError := engineInstance.buildTransaction.Commit(ctx); commitError != nil {
			return fmt.Errorf("failed to publish build transaction: %w", commitError)
		}

		engineInstance.cleanupOrphans()

		engineInstance.Deps.Metrics.RecordEnd()
		engineInstance.Deps.Logger.Info("Build complete")
		engineInstance.Deps.Metrics.Print()

		return nil
	})
}

// Build runs a full build with concurrency and locking safeguards.
func (engineInstance *Engine) Build(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer engineInstance.buildWaitGroup.Wait()
	defer cancel()

	// Prevent concurrent builds
	engineInstance.State.BuildMu.Lock()
	defer engineInstance.State.BuildMu.Unlock()

	// Reset per-build metrics so watch-mode rebuilds don't accumulate counters.
	if engineInstance.Deps.Metrics != nil {
		engineInstance.Deps.Metrics.Reset()
	}

	// Acquire build lock to prevent concurrent builds (skip in tests)
	var buildLock *fspkg.FileLock
	var lockErr error
	if !engineInstance.Ctx.IsTesting {
		buildLock, lockErr = fspkg.AcquireBuildLock(engineInstance.Cfg.OutputDir)
		if lockErr != nil {
			if !engineInstance.Cfg.ShouldForceLock {
				return fmt.Errorf("could not acquire build lock: %w (use --force-lock to override)", lockErr)
			}
			engineInstance.Deps.Logger.Warn("Acquiring build lock failed, but continuing due to --force-lock", "error", lockErr)
		} else {
			defer func() {
				if buildLock != nil {
					if err := buildLock.Release(); err != nil {
						engineInstance.Deps.Logger.Error("Failed to release build lock", "error", err)
					}
				}
			}()
		}
	}

	return engineInstance.BuildLocked(ctx)
}
