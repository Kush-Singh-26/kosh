package orchestration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
)

// buildSetupResult holds data from the initial setup phase.
type buildSetupResult struct {
	wasmWg             *sync.WaitGroup
	forceSocialRebuild bool
}

// setupPhase handles early build configuration and project-wide setup.
func (b *Engine) setupPhase(ctx context.Context) (*buildSetupResult, error) {
	if b.Deps.Metrics != nil {
		b.Deps.Metrics.Reset()
	}

	if b.Health != nil {
		b.Health.Reset()
	}

	// Always start each full build pass with a fresh session/tracking state.
	b.refreshBuildSession()

	// Check for cancellation early.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Project-wide setup.
	wasmWg := b.setupWasmDeployment(ctx)

	// Handle incremental social card rebuild if needed.
	forceSocialRebuild := b.checkSocialCardRebuild()

	// Warm up the JS renderer pool.
	b.initializeNativeRenderer(ctx)

	// Set dev build version.
	if b.Cfg.IsDev {
		b.Cfg.BuildVersion = time.Now().UnixNano()
	}

	// Pre-create output directories.
	if err := b.createOutputDirectories(); err != nil {
		return nil, err
	}

	return &buildSetupResult{
		wasmWg:             wasmWg,
		forceSocialRebuild: forceSocialRebuild,
	}, nil
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (b *Engine) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    b.Deps.Logger,
		Operation: "WASM compilation",
		Fn: func() error {
			updated, err := b.Deps.Wasm.CheckAndUpdate(ctx)
			if err == nil && updated {
				b.State.ForceGenerators.Store(true)
			}
			return err
		},
		Cleanup: func() {
			wasmWg.Done()
		},
	})
	return &wasmWg
}

// checkSocialCardRebuild determines if social cards need forced rebuild.
func (b *Engine) checkSocialCardRebuild() bool {
	if b.Cfg.ForceRebuild {
		return false
	}
	lastBuildTime := b.Tx.GetLastBuildTime()
	if lastBuildTime.IsZero() {
		return false
	}
	info, err := os.Stat("builder/generators/social.go")
	return err == nil && info.ModTime().After(lastBuildTime)
}

// initializeNativeRenderer warms up the JS renderer pool asynchronously.
func (b *Engine) initializeNativeRenderer(ctx context.Context) {
	if b.Deps.NativeRenderer != nil {
		go func() {
			b.Deps.NativeRenderer.EnsureInitialized(ctx)
		}()
	}
}

// createOutputDirectories creates required output directories.
func (b *Engine) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.Cfg.OutputDir, dir)); err != nil {
			b.Deps.Logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}
