package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils/async"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/errgroup"
)

// buildSetupResult holds data from the initial setup phase
type buildSetupResult struct {
	wasmWg             *sync.WaitGroup
	forceSocialRebuild bool
}

// buildAssetResult holds synchronization primitives for asset building
type buildAssetResult struct {
	assetsReady  <-chan struct{}
	assetWg      *sync.WaitGroup
	assetErrChan <-chan error
}

// buildScanResult holds channels for the parallel metadata scan
type buildScanResult struct {
	fileChan           <-chan models.ScannedFile
	scannerReady       <-chan struct{}
	metadataResultChan <-chan *models.MetadataScannerResult
	scannerErrChan     <-chan error
}

// setupPhase handles early build configuration and project-wide setup
func (b *Builder) setupPhase(ctx context.Context) (*buildSetupResult, error) {
	if b.metrics != nil {
		b.metrics.Reset()
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
	if b.cfg.IsDev {
		b.cfg.BuildVersion = time.Now().UnixNano()
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
func (b *Builder) assetPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildAssetResult {
	assetsReady, assetWg, assetErrChan := b.setupAssetBuilding(ctx, contentAssetsChan)

	// Tell the post service to wait for assets before entering render phase
	b.deps.Render.SetAssetsGate(assetsReady)

	return &buildAssetResult{
		assetsReady:  assetsReady,
		assetWg:      assetWg,
		assetErrChan: assetErrChan,
	}
}

// scanPhase launches the parallel metadata scanner
func (b *Builder) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	fileChan := make(chan models.ScannedFile, 1024)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, 1)
	scannerErrChan := make(chan error, 1)

	go func() {
		defer close(scannerReady)
		defer close(fileChan)
		defer close(contentAssetsChan)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerErr := b.deps.Scanner.Scan(ctx, b.cfg.ContentDir, b.SourceFs, b.cfg, fileChan)
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

// processPhase executes post processing and site-wide orchestration
func (b *Builder) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assets *buildAssetResult,
	scan *buildScanResult,
) error {
	// Set up site-wide generators
	runSiteWide, _ := b.setupSiteWideRendering(ctx, assets.assetsReady, setup.wasmWg, setup.forceSocialRebuild)

	// Process Posts
	var siteWideHas404 bool
	postResult, processErr := b.processPosts(ctx, b.cfg.ForceRebuild, setup.forceSocialRebuild, b.state.isCleanBuild, scan.fileChan)
	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Wait for scanner and assets
	metadataResult, scannerErr, assetErr := b.waitForScannerAndAssets(
		scan.scannerReady, scan.metadataResultChan, scan.scannerErrChan,
		assets.assetWg, assets.assetErrChan,
	)
	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Check if assets changed since last site-wide render
	assetsChanged := b.checkAssetsChanged(assets.assetsReady)

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
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404)
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (b *Builder) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	async.FireAndForgetWithCleanup(ctx, b.logger, "WASM compilation",
		func() error {
			return b.deps.Wasm.CheckAndUpdate(ctx)
		},
		func() {
			wasmWg.Done()
		})
	return &wasmWg
}

// checkSocialCardRebuild determines if social cards need forced rebuild
func (b *Builder) checkSocialCardRebuild() bool {
	if b.cfg.ForceRebuild {
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
func (b *Builder) initializeNativeRenderer(ctx context.Context) {
	if b.nativeRenderer != nil {
		b.nativeRenderer.EnsureInitialized(ctx)
	}
}

// createOutputDirectories creates required output directories
func (b *Builder) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.cfg.OutputDir, dir)); err != nil {
			b.logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}

// setupAssetBuilding starts asset building in a goroutine
func (b *Builder) setupAssetBuilding(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) (<-chan struct{}, *sync.WaitGroup, <-chan error) {
	slog.Info("Building assets...")
	assetTimer := timeutil.StartPhase("Asset building")
	b.deps.Render.SetAssets(map[string]string{})

	if setter, ok := b.deps.Asset.(interface {
		SetContentAssetsChannel(<-chan []models.ScannedAsset)
	}); ok {
		setter.SetContentAssetsChannel(contentAssetsChan)
	}

	assetsReady := make(chan struct{})
	b.deps.Asset.SetAssetsReadySignal(assetsReady)

	assetErrChan := make(chan error, 1)
	var assetWg sync.WaitGroup
	assetWg.Add(1)
	go func() {
		defer assetWg.Done()
		if err := b.copyStaticAndBuildAssets(ctx); err != nil {
			assetErrChan <- err
		}
		close(assetErrChan)
		assetTimer.Stop()
	}()

	return assetsReady, &assetWg, assetErrChan
}

// waitForScannerAndAssets waits for scanner and asset building to complete
func (b *Builder) waitForScannerAndAssets(
	scannerReady <-chan struct{},
	metadataResultChan <-chan *models.MetadataScannerResult,
	scannerErrChan <-chan error,
	assetWg *sync.WaitGroup,
	assetErrChan <-chan error,
) (*models.MetadataScannerResult, error, error) {
	<-scannerReady

	// Receive scanner result and error
	metadataResult := <-metadataResultChan
	scannerErr := <-scannerErrChan

	assetWg.Wait()
	var assetErr error
	select {
	case err := <-assetErrChan:
		assetErr = err
	default:
	}

	return metadataResult, scannerErr, assetErr
}

// waitForSiteWideRendering waits for site-wide generators and renders 404 if needed
func (b *Builder) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *timeutil.PhaseTimer, siteWideHas404 bool) error {
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
		if err := b.deps.Render.Render404(filepath.Join(b.cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: b.cfg.BaseURL, TabTitle: "404 Not Found",
			Config: b.cfg, RelativePrefix: "",
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}

// finalizeBuild writes post-build files and commits the transaction
func (b *Builder) finalizeBuild(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Write .nojekyll file
	if err := b.Sink.WriteFile(filepath.Join(b.cfg.OutputDir, ".nojekyll"), []byte{}); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}
	b.deps.Render.RegisterFile(filepath.Join(b.cfg.OutputDir, ".nojekyll"))

	// Sync/Commit transaction
	slog.Info("Publishing output...")
	syncTimer := timeutil.StartPhase("Publish")
	// Ensure WASM compilation and PWA generation finished before deploying and publishing
	wasmWg.Wait()

	// Reset ForceRebuild AFTER all async checks have completed
	b.cfg.ForceRebuild = false

	if err := b.deps.Wasm.Deploy(ctx, b.Tx.StagingDir()); err != nil {
		b.logger.Warn("Failed to deploy Search WASM", "error", err)
	}
	if err := b.Tx.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
}

// processPosts executes post processing and returns the result
func (b *Builder) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*services.PostResult, error) {
	return b.deps.Post.Process(ctx, shouldForce, forceSocialRebuild, outputMissing, fileChan)
}

// finalizePhase handles post-build cleanup and commit
func (b *Builder) finalizePhase(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Post-build files and commit
	if err := b.finalizeBuild(ctx, wasmWg); err != nil {
		return err
	}

	// Cleanup orphans (Dev mode only)
	b.CleanupOrphans()

	// Clear memory state
	b.deps.Render.ClearRenderedFiles()

	// Build complete
	b.metrics.RecordEnd()
	DevLogSuccess("Build complete")
	b.metrics.Print()

	return nil
}

// setupSiteWideRendering creates a function to run site-wide generators when metadata is ready.
func (b *Builder) setupSiteWideRendering(
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
		if cb.IndexedPosts != nil {
			b.mu.Lock()
			b.state.indexedPosts = cb.IndexedPosts
			b.mu.Unlock()
		}

		if b.shouldSkipSiteWideRendering(cb, assetsChanged) {
			return nil, nil
		}

		siteWideOnce.Do(func() {
			slog.Info("Rendering pagination, tags, metadata and PWA...")
			siteTimer = timeutil.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(ctx)

			// Pagination and Tags render HTML pages (need assets for CSS/JS paths)
			siteWideGroup.Go(func() error {
				b.waitForAssetsAvailability(siteWideCtx, assetsReady)
				return b.renderPagination(siteWideCtx, cb.AllPosts, cb.PinnedPosts, b.cfg.ForceRebuild)
			})
			siteWideGroup.Go(func() error {
				b.waitForAssetsAvailability(siteWideCtx, assetsReady)
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
				b.waitForAssetsAvailability(ctx, assetsReady)
				if err := b.generatePWA(ctx, b.cfg.ForceRebuild); err != nil {
					b.logger.Warn("PWA generation failed", "error", err)
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

// checkAssetsChanged computes a hash of the current asset map to detect changes
func (b *Builder) checkAssetsChanged(assetsReady <-chan struct{}) bool {
	b.waitForAssetsAvailability(context.Background(), assetsReady)
	assets := b.deps.Render.GetAssets()
	if len(assets) == 0 {
		return false
	}

	// Compute a simple hash of the asset map
	assetKeys := make([]string, 0, len(assets))
	for k := range assets {
		assetKeys = append(assetKeys, k)
	}
	sort.Strings(assetKeys)
	hasher := xxh3.New()
	for _, k := range assetKeys {
		_, _ = hasher.WriteString(k)
		_, _ = hasher.WriteString(assets[k])
	}
	currentAssetHash := hasher.Sum64()

	changed := currentAssetHash != b.state.lastAssetHash
	if changed {
		b.state.lastAssetHash = currentAssetHash
	}
	return changed
}

// shouldSkipSiteWideRendering determines if site-wide generators can be skipped
func (b *Builder) shouldSkipSiteWideRendering(cb *services.MetadataContext, assetsChanged bool) bool {
	useStaging := !b.cfg.IsDev || b.state.isCleanBuild
	if cb.AnyPostChanged || b.state.isCleanBuild || useStaging || b.state.forceGenerators.Load() || assetsChanged {
		b.state.forceGenerators.Store(false)
		return false
	}
	return true
}

func (b *Builder) waitForAssetsAvailability(ctx context.Context, assetsReady <-chan struct{}) {
	if len(b.deps.Render.GetAssets()) > 0 {
		return
	}
	if assetsReady == nil {
		return
	}
	select {
	case <-assetsReady:
	case <-ctx.Done():
	}
}

func (b *Builder) renderSiteMetadata(allPosts []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, assetsReady <-chan struct{}) error {
	g := new(errgroup.Group)

	// Sitemap - only on early call (indexedPosts == nil) or if allPosts provided
	if b.cfg.Features.Generators.Sitemap && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateSitemap(b.Sink, b.cfg.BaseURL, allPosts, tagMap, filepath.Join(b.cfg.OutputDir, "sitemap/sitemap.xml"))
			if err == nil {
				b.deps.Render.RegisterFile(filepath.Join(b.cfg.OutputDir, "sitemap/sitemap.xml"))
			} else {
				b.logger.Error("Failed to generate sitemap", "error", err)
				return err
			}
			return nil
		})
	}

	// RSS - only on early call
	if b.cfg.Features.Generators.RSS && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateRSS(b.Sink, b.cfg.BaseURL, allPosts, b.cfg.Title, b.cfg.Description, filepath.Join(b.cfg.OutputDir, "rss.xml"))
			if err == nil {
				b.deps.Render.RegisterFile(filepath.Join(b.cfg.OutputDir, "rss.xml"))
			} else {
				b.logger.Error("Failed to generate RSS feed", "error", err)
				return err
			}
			return nil
		})
	}

	// Search Index - only when indexedPosts provided
	if b.cfg.Features.Generators.Search && indexedPosts != nil {
		g.Go(func() error {
			_, err := generators.GenerateSearchIndex(b.Sink, b.cfg.OutputDir, indexedPosts)
			if err == nil {
				b.deps.Render.RegisterFile(filepath.Join(b.cfg.OutputDir, "search.bin"))
			} else {
				b.logger.Error("Failed to generate search index", "error", err)
				return err
			}
			return nil
		})
	}

	// Knowledge Graph - only on early call
	if b.cfg.Features.Generators.Graph && len(allPosts) > 0 {
		g.Go(func() error {
			_, err := generators.GenerateGraph(b.Sink, b.cfg.BaseURL, allPosts, filepath.Join(b.cfg.OutputDir, "graph.json"))
			if err != nil {
				b.logger.Error("Failed to generate knowledge graph data", "error", err)
			}

			// Render the HTML shell page — needs assets for CSS/JS paths
			if assetsReady != nil {
				<-assetsReady
			}
			if err := b.deps.Render.RenderGraph(filepath.Join(b.cfg.OutputDir, "graph.html"), models.PageData{
				Title:          "Graph View",
				TabTitle:       "Knowledge Graph | " + b.cfg.Title,
				BaseURL:        b.cfg.BaseURL,
				BuildVersion:   b.cfg.BuildVersion,
				Config:         b.cfg,
				RelativePrefix: "",
			}); err != nil {
				return fmt.Errorf("failed to render graph page: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}
