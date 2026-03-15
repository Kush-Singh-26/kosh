// Package run orchestrates full and incremental site builds.
// 
// Build orchestration call chain:
//   Build() → processPosts() → runParsePhase() → parseWorkerTask() → PostService.Process()
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
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/zeebo/xxh3"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/Kush-Singh-26/kosh/builder/utils/tx"
)

// refreshBuildSession creates a fresh Transaction and Sink for a new build pass.
// This ensures orphan detection correctly identifies files that were not written in the current pass.
func (b *Builder) refreshBuildSession() {
	// If we already have a sink/tx (e.g. injected in tests), don't overwrite it
	if b.Sink == nil || !utils.TestingMode {
		useStaging := !b.cfg.IsDev || b.state.isCleanBuild
		b.Tx = tx.NewBuildTransaction(b.cfg.OutputDir, useStaging)
		b.Sink = utils.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)
	}

	// Consolidated service reconfiguration - single explicit call per service
	b.deps.Post.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.deps.Asset.ReconfigureForBuild(b.Sink, b.SourceFs)
	b.deps.Render.ReconfigureForBuild(b.Sink, b.SourceFs)
}

// build executes the build logic without locking (internal use)
func (b *Builder) build(ctx context.Context) error {
	if b.metrics != nil {
		b.metrics.Reset()
	}

	// Always start each full build pass with a fresh session/tracking state
	b.refreshBuildSession()

	// Check for cancellation early
	select {
	case <-ctx.Done():
		return ctx.Err()
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
		return err
	}

	// Static Assets + Metadata Scanner + Post Parsing overlap
	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	assetsReady, assetWg, assetErrChan := b.setupAssetBuilding(ctx, contentAssetsChan)

	// Tell the post service to wait for assets before entering render phase
	b.deps.Render.SetAssetsGate(assetsReady)

	// Set up site-wide generators as an explicit function (called when metadata is ready)
	runSiteWide, _ := b.setupSiteWideRendering(ctx, assetsReady, wasmWg, forceSocialRebuild)

	// Run metadata scanner in parallel
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

	// Process Posts — parse phase overlaps with scanning and asset building
	// PostResult contains metadata needed for site-wide generators
	var siteWideHas404 bool
	postResult, processErr := b.processPosts(ctx, b.cfg.ForceRebuild, forceSocialRebuild, b.state.isCleanBuild, fileChan)
	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Wait for scanner and assets to complete
	metadataResult, scannerErr, assetErr := b.waitForScannerAndAssets(scannerReady, metadataResultChan, scannerErrChan, assetWg, assetErrChan)
	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Run site-wide generators explicitly with PostResult metadata
	// This provides clear orchestration while maintaining overlap with post rendering
	metadataCtx := &services.MetadataContext{
		AllPosts: postResult.AllPosts,
		PinnedPosts: postResult.PinnedPosts,
		TagMap: postResult.TagMap,
		IndexedPosts: postResult.IndexedPosts,
		AnyPostChanged: postResult.AnyPostChanged,
	}
	siteWideGroup, siteTimer := runSiteWide(metadataCtx)
	
	// Wait for site-wide rendering to complete
	if err := b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404); err != nil {
		return err
	}

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

// setupWasmDeployment launches WASM deployment asynchronously
func (b *Builder) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	go func() {
		defer wasmWg.Done()
		b.checkWasmUpdate(ctx)
	}()
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
	assetTimer := utils.StartPhase("Asset building")
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

// setupSiteWideRendering creates a function to run site-wide generators when metadata is ready.
// This provides explicit orchestration while allowing overlap with post rendering.
func (b *Builder) setupSiteWideRendering(
	ctx context.Context,
	assetsReady <-chan struct{},
	wasmWg *sync.WaitGroup,
	forceSocialRebuild bool,
) (func(*services.MetadataContext) (*errgroup.Group, *utils.PhaseTimer), *utils.PhaseTimer) {
	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *utils.PhaseTimer
	var siteWideOnce sync.Once

	// Return a function that runs site-wide generators with the provided metadata
	runSiteWide := func(cb *services.MetadataContext) (*errgroup.Group, *utils.PhaseTimer) {
		// Update in-memory cache for incremental search
		if cb.IndexedPosts != nil {
			b.mu.Lock()
			b.state.indexedPosts = cb.IndexedPosts
			b.mu.Unlock()
		}

		// Check if assets changed since last site-wide render
		b.waitForAssetsAvailability(ctx, assetsReady)
		assets := b.deps.Render.GetAssets()
		var currentAssetHash uint64
		if len(assets) > 0 {
			// Compute a simple hash of the asset map to detect changes
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
			currentAssetHash = hasher.Sum64()
		}

		// We must run site-wide generators if:
		// 1. Something changed (anyPostChanged)
		// 2. It's a clean build
		// 3. Assets have changed since last render
		// 4. Generators were explicitly forced (e.g. after deletion or on first build)
		useStaging := !b.cfg.IsDev || b.state.isCleanBuild
		assetsChanged := currentAssetHash != b.state.lastAssetHash
		if !cb.AnyPostChanged && !b.state.isCleanBuild && !useStaging && !b.state.forceGenerators.Load() && !assetsChanged {
			return nil, nil
		}
		b.state.forceGenerators.Store(false)
		b.state.lastAssetHash = currentAssetHash

		siteWideOnce.Do(func() {
			slog.Info("Rendering pagination, tags, metadata and PWA...")
			siteTimer = utils.StartPhase("Site-wide rendering")
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
			// PWA icon generation can be slow (200-400ms) and has no dependency on HTML renders
			wasmWg.Add(1)
			go func() {
				defer wasmWg.Done()
				b.waitForAssetsAvailability(ctx, assetsReady)
				_ = b.generatePWA(ctx, b.cfg.ForceRebuild)
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

// processPosts executes post processing and returns the result
func (b *Builder) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*services.PostResult, error) {
	return b.deps.Post.Process(ctx, shouldForce, forceSocialRebuild, outputMissing, fileChan)
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
func (b *Builder) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *utils.PhaseTimer, siteWideHas404 bool) error {
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

	// Reset ForceRebuild AFTER all async checks have completed
	b.cfg.ForceRebuild = false

	// Sync/Commit transaction
	slog.Info("Publishing output...")
	syncTimer := utils.StartPhase("Publish")
	// Ensure WASM deployment finished before publishing (it runs in parallel since step 1)
	wasmWg.Wait()
	if err := b.Tx.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
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

func (b *Builder) buildAssetOnly(ctx context.Context) error {
	if b.metrics != nil {
		b.metrics.Reset()
	}
	b.deps.Render.SetAssets(map[string]string{})

	slog.Info("Building assets...")
	assetTimer := utils.StartPhase("Asset building")

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
		_, _ = b.deps.Scanner.Scan(ctx, b.cfg.ContentDir, b.SourceFs, b.cfg, fileChan)
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
	var buildLock *utils.FileLock
	var lockErr error
	if !utils.TestingMode {
		buildLock, lockErr = utils.AcquireBuildLock(b.cfg.OutputDir)
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
