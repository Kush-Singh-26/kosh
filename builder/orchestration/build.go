// Package orchestration orchestrates full and incremental site builds.
//
// Build orchestration call chain:
//
//	Build() → processPhase() → PostService.ProcessStreaming()
//
// Build() coordinates high-level phases; individual Content processing,
// parallelism, progress tracking, and error isolation are handled by the
// Content service's streaming worker pipeline.
package orchestration

import (
	"context"
	"fmt"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/models"
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

// ReloadConfig reloads the configuration from disk.
func (engineInstance *Engine) ReloadConfig(ctx context.Context) error {
	engineInstance.State.BuildMu.Lock()
	defer engineInstance.State.BuildMu.Unlock()

	if err := engineInstance.Cfg.Reload(engineInstance.Deps.SourceFs); err != nil {
		return err
	}

	// Force clean build to invalidate fragment caches (navbar, footer) and regenerate everything
	engineInstance.State.IsCleanBuild = true
	engineInstance.State.ForceGenerators.Store(true)
	engineInstance.State.ForceRerender.Store(true)

	// Refresh build session to pick up new config fields
	engineInstance.refreshBuildSession(ctx)

	DevLogSuccess("Configuration reloaded, triggering full rebuild")

	return nil
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
