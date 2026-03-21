package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
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
	if b.Metrics != nil {
		b.Metrics.Reset()
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
	assetsReady, discoveryReady, assetWg, assetErrChan := b.Assets.SetupBuilding(ctx, contentAssetsChan)

	return &buildAssetResult{
		assetsReady:    assetsReady,
		discoveryReady: discoveryReady,
		assetWg:        assetWg,
		assetErrChan:   assetErrChan,
	}
}

// scanPhase launches the parallel metadata scanner
func (b *Engine) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	fileChan := make(chan models.ScannedFile, 1024)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, 1)
	scannerErrChan := make(chan error, 1)

	go func() {
		defer close(scannerReady)
		defer close(fileChan)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerErr := b.Deps.Scanner.Scan(ctx, b.Cfg.ContentDir, b.SourceFs, b.Cfg, fileChan)
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
	// Wait for scanner and discovery BEFORE rendering posts.
	// This ensures the image/WebP rewrite map is populated so HTML can reference
	// the correct paths. Image compression continues in the background while posts render.
	metadataResult, discoveryReady, scannerErr, assetErr, _ := b.waitForScannerAndAssets(
		scan.scannerReady, scan.metadataResultChan, scan.scannerErrChan,
		assets.assetWg, assets.assetErrChan, assets.discoveryReady,
	)
	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}

	var siteWideHas404 bool
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Set up site-wide generators (need full asset completion)
	runSiteWide, _ := b.setupSiteWideRendering(ctx, assets.assetsReady, setup.wasmWg, setup.forceSocialRebuild)

	// Wait for discovery signal so image rewrite map is ready before post-processing
	if discoveryReady != nil {
		<-discoveryReady
	}

	// Process Posts (render HTML with WebP image rewrites)
	postResult, processErr := b.processPosts(ctx, b.Cfg.ForceRebuild, setup.forceSocialRebuild, b.State.IsCleanBuild, metadataResult.Files)
	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Check if assets changed since last site-wide render
	assetsChanged := b.Assets.CheckChanged(ctx, assets.assetsReady)

	// Site-wide generators
	metadataCtx := &services.MetadataContext{
		AllPosts:       postResult.AllPosts,
		PinnedPosts:    postResult.PinnedPosts,
		TagMap:         postResult.TagMap,
		IndexedPosts:   postResult.IndexedPosts,
		AnyPostChanged: postResult.AnyPostChanged,
	}
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || b.Deps.Render.Has404Template())
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (b *Engine) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	async.FireAndForgetWithCleanup(ctx, b.Logger, "WASM compilation",
		func() error {
			return b.Deps.Wasm.CheckAndUpdate(ctx)
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

// initializeNativeRenderer warms up the JS renderer pool
func (b *Engine) initializeNativeRenderer(ctx context.Context) {
	if b.NativeRenderer != nil {
		b.NativeRenderer.EnsureInitialized(ctx)
	}
}

// createOutputDirectories creates required output directories
func (b *Engine) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.Cfg.OutputDir, dir)); err != nil {
			b.Logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}

// waitForScannerAndAssets waits for scanner and asset building to complete.
// The discoveryReady signal unblocks post-processing while image compression continues.
func (b *Engine) waitForScannerAndAssets(
	scannerReady <-chan struct{},
	metadataResultChan <-chan *models.MetadataScannerResult,
	scannerErrChan <-chan error,
	assetWg *sync.WaitGroup,
	assetErrChan <-chan error,
	discoveryReady <-chan struct{},
) (*models.MetadataScannerResult, <-chan struct{}, error, error, error) {
	<-scannerReady

	// Receive scanner result and error
	metadataResult := <-metadataResultChan
	scannerErr := <-scannerErrChan

	// Return discoveryReady separately so post-processing can unblock on it.
	// The caller will wait for assetWg separately if needed.
	discoverySignal := discoveryReady

	var assetErr error
	select {
	case err := <-assetErrChan:
		assetErr = err
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

	if siteWideHas404 {
		if err := b.Deps.Render.Render404(filepath.Join(b.Cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: "", TabTitle: "404 Not Found",
			Config: b.Cfg, RelativePrefix: "/",
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}

// finalizeBuild writes post-build files and commits the transaction
func (b *Engine) finalizeBuild(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Write .nojekyll file
	if err := b.Sink.WriteFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"), []byte{}); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}
	b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"))

	// Sync/Commit transaction
	b.Logger.Info("Publishing output...")
	syncTimer := timeutil.StartPhase("Publish")
	// Ensure WASM compilation and PWA generation finished before deploying and publishing
	wasmWg.Wait()

	// Reset ForceRebuild AFTER all async checks have completed
	b.Cfg.ForceRebuild = false

	if err := b.Deps.Wasm.Deploy(ctx, b.Sink); err != nil {
		b.Logger.Warn("Failed to deploy Search WASM", "error", err)
	}
	if err := b.Tx.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
}

// processPosts executes post processing and returns the result
func (b *Engine) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, files []models.ScannedFile) (*services.PostResult, error) {
	return b.Deps.Post.Process(ctx, shouldForce, forceSocialRebuild, outputMissing, files)
}

// finalizePhase handles post-build cleanup and commit
func (b *Engine) finalizePhase(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Post-build files and commit
	if err := b.finalizeBuild(ctx, wasmWg); err != nil {
		return err
	}

	// Cleanup orphans (Dev mode only)
	b.cleanupOrphans()

	// Clear memory state
	b.Deps.Render.ClearRenderedFiles()

	// Build complete
	b.Metrics.RecordEnd()
	b.Logger.Info("Build complete")
	b.Metrics.Print()

	return nil
}

// setupSiteWideRendering creates a function to run site-wide generators when metadata is ready.
func (b *Engine) setupSiteWideRendering(
	ctx context.Context,
	assetsReady <-chan struct{},
	wasmWg *sync.WaitGroup,
	forceSocialRebuild bool,
) (func(*services.MetadataContext, bool) (*errgroup.Group, *timeutil.PhaseTimer), *timeutil.PhaseTimer) {
	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *timeutil.PhaseTimer
	var siteWideOnce sync.Once

	// Return a function that runs site-wide generators with the provided metadata
	runSiteWide := func(cb *services.MetadataContext, assetsChanged bool) (*errgroup.Group, *timeutil.PhaseTimer) {
		// Update in-memory cache for incremental search
		if b.Search != nil && cb.IndexedPosts != nil {
			b.Search.SetIndexedPosts(cb.IndexedPosts)
		}

		if b.shouldSkipSiteWideRendering(cb, assetsChanged) {
			return nil, nil
		}

		siteWideOnce.Do(func() {
			b.Logger.Info("Rendering pagination, tags, metadata and PWA...")
			siteTimer = timeutil.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(ctx)

			// Pagination and Tags render HTML pages (need assets for CSS/JS paths)
			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderPagination(siteWideCtx, cb.AllPosts, cb.PinnedPosts, b.Cfg.ForceRebuild)
			})
			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderTags(siteWideCtx, cb.TagMap, forceSocialRebuild)
			})
			// Sitemap, RSS, Search are pure data generators (no HTML assets needed)
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(cb.AllPosts, cb.TagMap, nil, assetsReady)
			})
			// PWA icon generation can be slow (200-400ms) and has no dependency on HTML renders.
			wasmWg.Add(1)
			go func() {
				defer wasmWg.Done()
				b.Assets.WaitForAvailability(ctx, assetsReady)
				if err := b.generatePWA(ctx, b.Cfg.ForceRebuild); err != nil {
					b.Logger.Warn("PWA generation failed", "error", err)
				}
			}()
		})

		// Handle search index specifically on the second call (when indexedPosts available)
		if cb.IndexedPosts != nil {
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(nil, nil, cb.IndexedPosts, nil)
			})
		}

		return siteWideGroup, siteTimer
	}

	return runSiteWide, nil
}

// shouldSkipSiteWideRendering determines if site-wide generators can be skipped
func (b *Engine) shouldSkipSiteWideRendering(cb *services.MetadataContext, assetsChanged bool) bool {
	useStaging := !b.Cfg.IsDev || b.State.IsCleanBuild
	if cb.AnyPostChanged || b.State.IsCleanBuild || useStaging || b.State.ForceGenerators.Load() || assetsChanged {
		b.State.ForceGenerators.Store(false)
		return false
	}
	return true
}

func (b *Engine) renderSiteMetadata(allPosts []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, assetsReady <-chan struct{}) error {
	g := new(errgroup.Group)

	// Sitemap - only on early call (indexedPosts == nil) or if allPosts provided
	if b.Cfg.Features.Generators.Sitemap && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateSitemap(b.Sink, b.Cfg.BaseURL, allPosts, tagMap, filepath.Join(b.Cfg.OutputDir, "sitemap/sitemap.xml"))
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "sitemap/sitemap.xml"))
			} else {
				b.Logger.Error("Failed to generate sitemap", "error", err)
				return err
			}
			return nil
		})

		g.Go(func() error {
			_, err := generators.GenerateRobotsTxt(b.Sink, b.Cfg.BaseURL, filepath.Join(b.Cfg.OutputDir, "robots.txt"))
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "robots.txt"))
			} else {
				b.Logger.Error("Failed to generate robots.txt", "error", err)
				return err
			}
			return nil
		})
	}

	// RSS - only on early call
	if b.Cfg.Features.Generators.RSS && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateRSS(b.Sink, b.Cfg.BaseURL, allPosts, b.Cfg.Title, b.Cfg.Description, filepath.Join(b.Cfg.OutputDir, "rss.xml"))
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "rss.xml"))
			} else {
				b.Logger.Error("Failed to generate RSS feed", "error", err)
				return err
			}
			return nil
		})
	}

	// Search Index - only when indexedPosts provided
	if b.Cfg.Features.Generators.Search && indexedPosts != nil {
		g.Go(func() error {
			searchPath, err := generators.GenerateSearchIndex(b.Sink, indexedPosts)
			if err == nil {
				b.Deps.Render.RegisterFile(searchPath)
			} else {
				b.Logger.Error("Failed to generate search index", "error", err)
				return err
			}
			return nil
		})
	}

	// Knowledge Graph - only on early call
	if b.Cfg.Features.Generators.Graph && len(allPosts) > 0 {
		g.Go(func() error {
			_, err := generators.GenerateGraph(b.Sink, b.Cfg.BaseURL, allPosts, filepath.Join(b.Cfg.OutputDir, "graph.json"))
			if err != nil {
				b.Logger.Error("Failed to generate knowledge graph data", "error", err)
			}

			// Render the HTML shell page — needs assets for CSS/JS paths
			if assetsReady != nil {
				<-assetsReady
			}
			if err := b.Deps.Render.RenderGraph(filepath.Join(b.Cfg.OutputDir, "graph.html"), models.PageData{
				Title:          "Graph View",
				TabTitle:       "Knowledge Graph | " + b.Cfg.Title,
				BaseURL:        "",
				BuildVersion:   b.Cfg.BuildVersion,
				Config:         b.Cfg,
				RelativePrefix: "/",
			}); err != nil {
				return fmt.Errorf("failed to render graph page: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}
