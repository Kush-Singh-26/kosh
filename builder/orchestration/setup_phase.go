package orchestration

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/data"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// buildSetupResult holds data from the initial setup phase.
type buildSetupResult struct {
	wasmWg             *sync.WaitGroup
	forceSocialRebuild bool
}

// setupPhase handles early build configuration and project-wide setup.
func (engineInstance *Engine) setupPhase(ctx context.Context) (*buildSetupResult, error) {
	if engineInstance.Deps.Metrics != nil {
		engineInstance.Deps.Metrics.Reset()
	}

	if engineInstance.Health != nil {
		engineInstance.Health.Reset()
	}

	// Always start each full build pass with a fresh session/tracking state.
	engineInstance.refreshBuildSession()

	// Check for cancellation early.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Project-wide setup.
	wasmWaitGroup := engineInstance.setupWasmDeployment(ctx)

	// Handle incremental social card rebuild if needed.
	forceSocialRebuild := engineInstance.checkSocialCardRebuild()

	// Warm up the JS renderer pool.
	engineInstance.initializeNativeRenderer(ctx)

	// Set dev build version if not already set.
	if engineInstance.Cfg.IsDev && engineInstance.Cfg.BuildVersion == 0 {
		engineInstance.Cfg.BuildVersion = time.Now().UnixNano()
	}

	// Pre-create output directories.
	if err := engineInstance.createOutputDirectories(); err != nil {
		return nil, err
	}

	// Load site data from data/ directory
	dataDir := filepath.Join(engineInstance.Cfg.SiteRoot, "data")
	siteData, err := data.Load(engineInstance.Deps.SourceFs, dataDir)
	if err != nil {
		engineInstance.Deps.Logger.Error("Failed to load site data", "dir", dataDir, "error", err)
		// We continue even if data loading fails, it might be empty or optional
	}
	engineInstance.Cfg.SiteData = siteData

	// Warm up the fragment cache for common UI components.
	engineInstance.warmupFragmentCache(ctx)

	return &buildSetupResult{
		wasmWg:             wasmWaitGroup,
		forceSocialRebuild: forceSocialRebuild,
	}, nil
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (engineInstance *Engine) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWaitGroup sync.WaitGroup
	wasmWaitGroup.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    engineInstance.Deps.Logger,
		Operation: "WASM compilation",
		Fn: func() error {
			updated, err := engineInstance.Deps.Wasm.CheckAndUpdate(ctx)
			if err == nil && updated {
				engineInstance.State.ForceGenerators.Store(true)
			}
			return err
		},
		Cleanup: func() {
			wasmWaitGroup.Done()
		},
	})
	return &wasmWaitGroup
}

// checkSocialCardRebuild determines if social cards need forced rebuild.
func (engineInstance *Engine) checkSocialCardRebuild() bool {
	if engineInstance.Cfg.ShouldForceRebuild {
		return false
	}
	lastBuildTime := engineInstance.buildTransaction.GetLastBuildTime()
	if lastBuildTime.IsZero() {
		return false
	}
	fileInfo, err := os.Stat("builder/generators/social.go")
	return err == nil && fileInfo.ModTime().After(lastBuildTime)
}

// initializeNativeRenderer warms up the JS renderer pool asynchronously.
func (engineInstance *Engine) initializeNativeRenderer(ctx context.Context) {
	if engineInstance.Deps.NativeRenderer != nil {
		logger := engineInstance.Deps.Logger
		if logger == nil {
			logger = slog.Default()
		}
		async.FireAndForget(ctx, logger, "native renderer warmup", func() error {
			engineInstance.Deps.NativeRenderer.EnsureInitialized(ctx)
			return nil
		})
	}
}

// createOutputDirectories creates required output directories.
func (engineInstance *Engine) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := engineInstance.artifactSink.MkdirAll(filepath.Join(engineInstance.Cfg.OutputDir, dir)); err != nil {
			engineInstance.Deps.Logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}

// warmupFragmentCache pre-renders common UI fragments to populate caches before workers start.
func (engineInstance *Engine) warmupFragmentCache(ctx context.Context) {
	if engineInstance.Deps.Render == nil {
		return
	}

	// Contexts and blocks that are site-wide and expensive to render first-time
	commonContexts := []models.PageContext{models.ContextHome, models.ContextBlog}
	commonBlocks := []string{"navbar-identity", "footer"}

	// Mock data for warmup - PreparePageData will fill in the rest
	data := models.PageData{
		RelativePrefix: "",
	}

	for _, contextType := range commonContexts {
		data.Context = contextType
		for _, block := range commonBlocks {
			// Trigger render to populate memory and persistent caches
			_, _ = engineInstance.Deps.Render.RenderFragment(string(contextType), block, data)
		}
	}
}
