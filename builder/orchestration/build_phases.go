package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"golang.org/x/sync/errgroup"
)

// buildSetupResult holds data from the initial setup phase
type buildSetupResult struct {
	wasmWg             *sync.WaitGroup
	forceSocialRebuild bool
}

// buildAssetResult holds synchronization primitives for asset building
type buildAssetResult struct {
	assetsReady    <-chan struct{}
	discoveryReady <-chan struct{} // signals when image rewrite map is populated
	assetWg        *sync.WaitGroup
	assetErrChan   <-chan error
}

// buildScanResult holds channels for the parallel metadata scan
type buildScanResult struct {
	fileChan           <-chan models.ScannedFile
	scannerReady       <-chan struct{}
	metadataResultChan <-chan *models.MetadataScannerResult
	scannerErrChan     <-chan error
}

// setupPhase handles early build configuration and project-wide setup
func (b *Engine) setupPhase(ctx context.Context) (*buildSetupResult, error) {
	if b.Deps.Metrics != nil {
		b.Deps.Metrics.Reset()
	}

	if b.Health != nil {
		b.Health.Reset()
	}

	// Always start each full build pass with a fresh session/tracking state
	b.refreshBuildSession()

	// Check for cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Project-wide setup
	wasmWg := b.setupWasmDeployment(ctx)

	// Handle incremental social card rebuild if needed
	forceSocialRebuild := b.checkSocialCardRebuild()

	// Warm up the JS renderer pool
	b.initializeNativeRenderer(ctx)

	// Set dev build version
	if b.Cfg.IsDev {
		b.Cfg.BuildVersion = time.Now().UnixNano()
	}

	// Pre-create output directories
	if err := b.createOutputDirectories(); err != nil {
		return nil, err
	}

	return &buildSetupResult{
		wasmWg:             wasmWg,
		forceSocialRebuild: forceSocialRebuild,
	}, nil
}

// assetPhase starts the asset building pipeline
func (b *Engine) assetPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildAssetResult {
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.StartPhase(ui.PhaseAssets)
	}
	forceAssetBuild := b.Cfg.ForceRebuild
	assetsReady, discoveryReady, assetWg, assetErrChan := b.Assets.SetupBuilding(ctx, contentAssetsChan, forceAssetBuild)

	return &buildAssetResult{
		assetsReady:    assetsReady,
		discoveryReady: discoveryReady,
		assetWg:        assetWg,
		assetErrChan:   assetErrChan,
	}
}

// scanPhase launches the parallel metadata scanner
func (b *Engine) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.StartPhase(ui.PhaseScan)
	}
	fileChan := make(chan models.ScannedFile, 1024)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, 1)
	scannerErrChan := make(chan error, 1)

	go func() {
		defer close(scannerReady)
		defer close(fileChan)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerErr := b.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: b.Cfg.ContentDir,
			SrcFs:      b.Deps.SourceFs,
			Cfg:        b.Cfg,
			FileChan:   fileChan,
		})
		if scannerErr == nil {
			contentAssetsChan <- metadataResult.ContentAssets
		}
		// Always send result and error (even if nil)
		metadataResultChan <- metadataResult
		scannerErrChan <- scannerErr
	}()

	return &buildScanResult{
		fileChan:           fileChan,
		scannerReady:       scannerReady,
		metadataResultChan: metadataResultChan,
		scannerErrChan:     scannerErrChan,
	}
}

// processPhase executes post processing and site-wide orchestration.
// Assets (including image compression) MUST complete before post rendering,
// since post rendering rewrites PNG/JPG/JPEG image references to WebP.
func (b *Engine) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assets *buildAssetResult,
	scan *buildScanResult,
) error {
	// Start streaming post processing (ingesting from the scanner's fileChan)
	type postStreamRes struct {
		res *post.PostResult
		err error
	}
	postResChan := make(chan postStreamRes, 1)
	go func() {
		res, err := b.Deps.Post.ProcessStreaming(post.ProcessOptions{
			Ctx:                ctx,
			ShouldForce:        b.Cfg.ForceRebuild,
			ForceSocialRebuild: setup.forceSocialRebuild,
			OutputMissing:      b.State.IsCleanBuild,
			FileChan:           scan.fileChan,
		})
		postResChan <- postStreamRes{res, err}
	}()

	// Wait for scanner and discovery metadata (needed for ContentAssets and site-wide state).
	// This ensures the image/WebP rewrite map is populated so HTML can reference
	// the correct paths. Image compression continues in the background while posts render.
	metadataResult, discoverySignal, scannerErr, assetErr, _ := b.waitForScannerAndAssets(WaitScannerAssetsOptions{
		Ctx:                ctx,
		ScannerReady:       scan.scannerReady,
		MetadataResultChan: scan.metadataResultChan,
		ScannerErrChan:     scan.scannerErrChan,
		AssetWg:            assets.assetWg,
		AssetErrChan:       assets.assetErrChan,
		DiscoveryReady:     assets.discoveryReady,
	})

	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhaseScan, 0)
		b.Deps.Reporter.StartPhase(ui.PhasePosts)
	}

	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}

	// Discovery signal must be ready before templates can be safely executed by the renderer
	if discoverySignal != nil {
		select {
		case <-discoverySignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	var siteWideHas404 bool
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Set up site-wide generators (need full asset completion)
	runSiteWide, _ := b.setupSiteWideRendering(SiteWideOptions{
		Ctx:                ctx,
		AssetsReady:        assets.assetsReady,
		WasmWg:             setup.wasmWg,
		ForceSocialRebuild: setup.forceSocialRebuild,
	})

	// Wait for post processing to finish (it was started as a stream)
	var postResult *post.PostResult
	var processErr error
	select {
	case pr := <-postResChan:
		postResult = pr.res
		processErr = pr.err
	case <-ctx.Done():
		return ctx.Err()
	}

	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhasePosts, 0)
		b.Deps.Reporter.StartPhase(ui.PhaseSiteWide)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Check if assets changed since last site-wide render
	assetsChanged := b.Assets.CheckChanged(ctx, assets.assetsReady)

	// Site-wide generators
	metadataCtx := postResult.ToMetadataContext()
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || b.Deps.Render.Has404Template())
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (b *Engine) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	async.FireAndForgetWithCleanup(ctx, b.Deps.Logger, "WASM compilation",
		func() error {
			updated, err := b.Deps.Wasm.CheckAndUpdate(ctx)
			if err == nil && updated {
				b.State.ForceGenerators.Store(true)
			}
			return err
		},
		func() {
			wasmWg.Done()
		})
	return &wasmWg
}

// checkSocialCardRebuild determines if social cards need forced rebuild
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

// initializeNativeRenderer warms up the JS renderer pool asynchronously
func (b *Engine) initializeNativeRenderer(ctx context.Context) {
	if b.Deps.NativeRenderer != nil {
		go func() {
			b.Deps.NativeRenderer.EnsureInitialized(ctx)
		}()
	}
}

// createOutputDirectories creates required output directories
func (b *Engine) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.Cfg.OutputDir, dir)); err != nil {
			b.Deps.Logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}

type WaitScannerAssetsOptions struct {
	Ctx                context.Context
	ScannerReady       <-chan struct{}
	MetadataResultChan <-chan *models.MetadataScannerResult
	ScannerErrChan     <-chan error
	AssetWg            *sync.WaitGroup
	AssetErrChan       <-chan error
	DiscoveryReady     <-chan struct{}
}

// waitForScannerAndAssets waits for scanner and asset building to complete.
// The discoveryReady signal unblocks post-processing while image compression continues.
func (b *Engine) waitForScannerAndAssets(opts WaitScannerAssetsOptions) (*models.MetadataScannerResult, <-chan struct{}, error, error, error) {
	ctx := opts.Ctx
	scannerReady := opts.ScannerReady
	metadataResultChan := opts.MetadataResultChan
	scannerErrChan := opts.ScannerErrChan
	assetErrChan := opts.AssetErrChan
	discoveryReady := opts.DiscoveryReady

	select {
	case <-scannerReady:
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	}

	// Receive scanner result and error
	var metadataResult *models.MetadataScannerResult
	var scannerErr error
	select {
	case metadataResult = <-metadataResultChan:
		scannerErr = <-scannerErrChan
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	}

	// Return discoveryReady separately so post-processing can unblock on it.
	// The caller will wait for assetWg separately if needed.
	discoverySignal := discoveryReady

	var assetErr error
	select {
	case err := <-assetErrChan:
		assetErr = err
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	default:
	}

	return metadataResult, discoverySignal, scannerErr, assetErr, nil
}

// waitForSiteWideRendering waits for site-wide generators and renders 404 if needed
func (b *Engine) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *timeutil.PhaseTimer, siteWideHas404 bool) error {
	if siteWideGroup == nil {
		return nil
	}

	if err := siteWideGroup.Wait(); err != nil {
		if siteTimer != nil {
			siteTimer.Stop()
		}
		return err
	}
	if siteTimer != nil {
		siteTimer.Stop()
	}
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhaseSiteWide, 0)
		b.Deps.Reporter.StartPhase(ui.PhasePublish)
	}

	if siteWideHas404 {
		if err := b.Deps.Render.Render404(filepath.Join(b.Cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: b.Cfg.BaseURL, TabTitle: "404 Not Found",
			Config: b.Cfg, RelativePrefix: "",
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}

// finalizeBuild writes post-build files and commits the transaction
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

type ProcessPostsOptions struct {
	Ctx                context.Context
	ShouldForce        bool
	ForceSocialRebuild bool
	OutputMissing      bool
	Files              []models.ScannedFile
}

// processPosts executes post processing and returns the result
func (b *Engine) processPosts(opts ProcessPostsOptions) (*post.PostResult, error) {
	return b.Deps.Post.Process(post.ProcessOptions{
		Ctx:                opts.Ctx,
		ShouldForce:        opts.ShouldForce,
		ForceSocialRebuild: opts.ForceSocialRebuild,
		OutputMissing:      opts.OutputMissing,
		Files:              opts.Files,
	})
}

// finalizePhase handles post-build cleanup and commit
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
