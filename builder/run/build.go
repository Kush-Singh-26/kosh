package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// build executes the build logic without locking (internal use)
func (b *Builder) build(ctx context.Context) error {
	if b.metrics != nil {
		b.metrics.Reset()
	}

	// Check for cancellation early
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 1. Project-wide setup
	// Launch WASM deployment asynchronously — it writes to staging dir and
	// has no dependencies on other build phases. We only need it complete
	// before Tx.Commit() publishes the staging directory.
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	go func() {
		defer wasmWg.Done()
		b.checkWasmUpdate()
	}()

	// Handle incremental social card rebuild if needed
	var forceSocialRebuild bool
	lastBuildTime := b.Tx.GetLastBuildTime()
	if !b.cfg.ForceRebuild && !lastBuildTime.IsZero() {
		// If generator binary or source changed, force social card update
		if info, err := os.Stat("builder/generators/social.go"); err == nil && info.ModTime().After(lastBuildTime) {
			forceSocialRebuild = true
		}
	}

	if b.cfg.IsDev {
		b.cfg.BuildVersion = time.Now().UnixNano()
	}

	// Pre-create output directories
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.cfg.OutputDir, dir)); err != nil {
			b.logger.Error("Failed to create directory", "dir", dir, "error", err)
		}
	}

	// 2. Static Assets + Metadata Scanner + Post Parsing overlap
	//
	// The parse phase (markdown parsing, KaTeX SSR, BM25 analysis) has no
	// dependency on asset building (image optimization, esbuild bundling).
	// Only the render phase needs the Assets map (hashed CSS/JS filenames).
	// We launch asset building in a goroutine and gate the render phase
	// inside Process() via the assetsReady channel.
	fmt.Println("📦 Building assets...")
	assetTimer := utils.StartPhase("Asset building")
	b.renderService.SetAssets(map[string]string{})

	assetsReady := make(chan struct{})
	b.assetService.SetAssetsReadySignal(assetsReady)

	var assetErr error
	go func() {
		if err := b.copyStaticAndBuildAssets(ctx); err != nil {
			assetErr = err
		}
		assetTimer.Stop()
	}()

	// Tell the post service to wait for assets before entering render phase
	b.postService.SetAssetsGate(assetsReady)

	// Set up the site-wide rendering callback. When post metadata becomes
	// available inside Process() (after parse, before render), this callback
	// fires the site-wide errgroup so it overlaps with the render phase.
	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *utils.PhaseTimer
	var siteWideHas404 bool
	var siteWideOnce sync.Once

	b.postService.SetMetadataCallback(func(
		cbAllPosts []models.PostMetadata,
		cbPinnedPosts []models.PostMetadata,
		cbTagMap map[string][]models.PostMetadata,
		cbIndexedPosts []models.IndexedPost,
		cbAnyChanged bool,
	) {
		// We must run site-wide generators if:
		// 1. Something changed (anyPostChanged)
		// 2. It's a clean build
		// 3. We are using staging (production build) - because staging starts empty
		//    and Commit() will replace the real output with staging contents.
		useStaging := !b.cfg.IsDev || b.isCleanBuild
		if !cbAnyChanged && !b.isCleanBuild && !useStaging {
			return
		}

		siteWideOnce.Do(func() {
			fmt.Println("📄 Rendering pagination, tags, metadata and PWA...")
			siteTimer = utils.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(ctx)

			// Pagination and Tags render HTML pages (need assets for CSS/JS paths).
			// Proceed once hashed assets are available (or build channel closes).
			siteWideGroup.Go(func() error {
				b.waitForAssetsAvailability(siteWideCtx, assetsReady)
				return b.renderPagination(siteWideCtx, cbAllPosts, cbPinnedPosts, b.cfg.ForceRebuild)
			})
			siteWideGroup.Go(func() error {
				b.waitForAssetsAvailability(siteWideCtx, assetsReady)
				return b.renderTags(siteWideCtx, cbTagMap, forceSocialRebuild)
			})
			// Sitemap, RSS, Search are pure data generators (no HTML assets needed).
			// Graph HTML rendering needs assets — renderSiteMetadata waits internally.
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(cbAllPosts, cbTagMap, cbIndexedPosts, assetsReady)
			})
			// PWA: SW needs assets, manifest and icons don't.
			siteWideGroup.Go(func() error {
				b.waitForAssetsAvailability(siteWideCtx, assetsReady)
				return b.generatePWA(siteWideCtx, b.cfg.ForceRebuild)
			})
		})

		// Handle search index specifically on the second call (when indexedPosts available)
		if cbIndexedPosts != nil {
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(nil, nil, cbIndexedPosts, nil)
			})
		}
	})

	var metadataResult *services.MetadataScannerResult
	var scannerErr error

	// Run metadata scanner in parallel with both assets and parsing
	scannerReady := make(chan struct{})
	go func() {
		defer close(scannerReady)
		metadataResult, scannerErr = b.metadataScanner.Scan(ctx, b.cfg.ContentDir, b.SourceFs, b.cfg)
	}()

	// Wait for scanner (needed before post processing starts)
	<-scannerReady
	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if setter, ok := b.assetService.(interface{ SetContentAssets([]services.ScannedAsset) }); ok {
		setter.SetContentAssets(metadataResult.ContentAssets)
	}

	// 3. Process Posts — parse phase overlaps with asset building,
	//    render phase gates on assetsReady channel inside Process().
	//    Site-wide tasks start via the metadata callback above, overlapping
	//    with the render phase for faster cold builds.
	_, _, _, _, _, has404, err := b.processPosts(ctx, b.cfg.ForceRebuild, forceSocialRebuild, b.isCleanBuild, metadataResult)
	if err != nil {
		return fmt.Errorf("post processing failed: %w", err)
	}
	siteWideHas404 = has404

	// Ensure asset goroutine completed and check for errors
	<-assetsReady
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}

	// 4. Wait for site-wide rendering to complete (started by metadata callback
	//    inside Process(), overlapping with the render phase)
	if siteWideGroup != nil {
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
			b.renderService.Render404(filepath.Join(b.cfg.OutputDir, "404.html"), models.PageData{
				Title: "404 Not Found", BaseURL: b.cfg.BaseURL, TabTitle: "404 Not Found",
				Config: b.cfg, RelativePrefix: "",
			})
		}
	}

	// 5. Post-build files
	_ = b.Sink.WriteFile(filepath.Join(b.cfg.OutputDir, ".nojekyll"), []byte{})
	b.renderService.RegisterFile(filepath.Join(b.cfg.OutputDir, ".nojekyll"))

	// Reset ForceRebuild AFTER all async checks have completed
	b.cfg.ForceRebuild = false

	fmt.Println("💾 Publishing output...")
	syncTimer := utils.StartPhase("Publish")
	// Ensure WASM deployment finished before publishing (it runs in parallel since step 1)
	wasmWg.Wait()
	if err := b.Tx.Commit(); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	// Clear memory state
	b.renderService.ClearRenderedFiles()

	// Build complete
	b.metrics.RecordEnd()
	fmt.Printf("\n✨ Build complete!\n")
	b.metrics.Print()

	return nil
}

func (b *Builder) waitForAssetsAvailability(ctx context.Context, assetsReady <-chan struct{}) {
	if len(b.renderService.GetAssets()) > 0 {
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
	b.renderService.SetAssets(map[string]string{})

	fmt.Println("📦 Building assets...")
	assetTimer := utils.StartPhase("Asset building")
	b.Tx = utils.NewBuildTransaction(b.cfg.OutputDir, false)
	b.Sink = utils.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)
	b.renderService.SetSink(b.Sink)
	b.assetService.SetSink(b.Sink)
	b.postService.SetSink(b.Sink)

	assets, err := b.assetService.BuildForAssetChange(ctx)
	assetTimer.Stop()
	if err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}

	b.renderService.SetAssets(assets)
	b.renderService.ClearRenderedFiles()
	b.postService.SetAssetsGate(nil)
	metadataResult, scanErr := b.metadataScanner.Scan(ctx, b.cfg.ContentDir, b.SourceFs, b.cfg)
	if scanErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scanErr)
	}

	shouldForce := false
	forceSocialRebuild := false
	outputMissing := false
	_, _, _, _, _, _, err = b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing, metadataResult)
	if err != nil {
		return fmt.Errorf("post processing failed: %w", err)
	}

	if err := b.Tx.Commit(); err != nil {
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}

	b.metrics.RecordEnd()
	fmt.Printf("\n✨ Build complete!\n")
	b.metrics.Print()
	return nil
}

func (b *Builder) Build(ctx context.Context) error {
	// Prevent concurrent builds
	b.buildMu.Lock()
	defer b.buildMu.Unlock()

	// Reset per-build metrics so watch-mode rebuilds don't accumulate counters.
	if b.metrics != nil {
		b.metrics.Reset()
	}

	// Setup Build Transaction
	useStaging := !b.cfg.IsDev || b.isCleanBuild
	if b.Tx == nil {
		b.Tx = utils.NewBuildTransaction(b.cfg.OutputDir, useStaging)
	}
	if b.Sink == nil {
		b.Sink = utils.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)
	}

	// Inject sink into services
	b.postService.SetSink(b.Sink)
	b.assetService.SetSink(b.Sink)
	b.renderService.SetSink(b.Sink)

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
	if err := b.assetService.Build(ctx); err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}
	return nil
}

func (b *Builder) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, earlyMetadata *services.MetadataScannerResult) ([]models.PostMetadata, []models.PostMetadata, map[string][]models.PostMetadata, []models.IndexedPost, bool, bool, error) {
	result, err := b.postService.Process(ctx, shouldForce, forceSocialRebuild, outputMissing, earlyMetadata)
	if err != nil {
		return nil, nil, nil, nil, false, false, err
	}
	return result.AllPosts, result.PinnedPosts, result.TagMap, result.IndexedPosts, result.AnyPostChanged, result.Has404, nil
}

func (b *Builder) renderSiteMetadata(allPosts []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, assetsReady <-chan struct{}) error {
	g := new(errgroup.Group)

	// Sitemap - only on early call (indexedPosts == nil) or if allPosts provided
	if b.cfg.Features.Generators.Sitemap && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateSitemap(b.Sink, b.cfg.BaseURL, allPosts, tagMap, filepath.Join(b.cfg.OutputDir, "sitemap/sitemap.xml"))
			if err == nil {
				b.renderService.RegisterFile(filepath.Join(b.cfg.OutputDir, "sitemap/sitemap.xml"))
			} else {
				b.logger.Error("Failed to generate sitemap", "error", err)
			}
			return nil
		})
	}

	// RSS - only on early call
	if b.cfg.Features.Generators.RSS && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateRSS(b.Sink, b.cfg.BaseURL, allPosts, b.cfg.Title, b.cfg.Description, filepath.Join(b.cfg.OutputDir, "rss.xml"))
			if err == nil {
				b.renderService.RegisterFile(filepath.Join(b.cfg.OutputDir, "rss.xml"))
			} else {
				b.logger.Error("Failed to generate RSS feed", "error", err)
			}
			return nil
		})
	}

	// Search Index - only when indexedPosts provided
	if b.cfg.Features.Generators.Search && indexedPosts != nil {
		g.Go(func() error {
			_, err := generators.GenerateSearchIndex(b.Sink, b.cfg.OutputDir, indexedPosts)
			if err == nil {
				b.renderService.RegisterFile(filepath.Join(b.cfg.OutputDir, "search.bin"))
			} else {
				b.logger.Error("Failed to generate search index", "error", err)
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
			b.renderService.RenderGraph(filepath.Join(b.cfg.OutputDir, "graph.html"), models.PageData{
				Title:          "Graph View",
				TabTitle:       "Knowledge Graph | " + b.cfg.Title,
				BaseURL:        b.cfg.BaseURL,
				BuildVersion:   b.cfg.BuildVersion,
				Config:         b.cfg,
				RelativePrefix: "",
			})
			return nil
		})
	}

	return g.Wait()
}
