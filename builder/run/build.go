package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// Build executes a single build pass
func (b *Builder) Build(ctx context.Context) error {
	b.buildMu.Lock()
	defer b.buildMu.Unlock()

	// Check for cancellation early
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Determine if we should use atomic staging directory
	// Always use staging for production builds, or if output is missing.
	outputMissing := false
	if _, err := os.Stat(b.cfg.OutputDir); os.IsNotExist(err) {
		outputMissing = true
	}
	useStaging := !b.cfg.IsDev || outputMissing
	b.isCleanBuild = useStaging // using staging implies we are starting clean
	if b.diagramAdapter != nil {
		b.diagramAdapter.SetPersistenceEnabled(!outputMissing)
	}

	b.Tx = utils.NewBuildTransaction(b.cfg.OutputDir, useStaging)
	b.Sink = utils.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)

	// Clean staging dir if it somehow exists from a crashed build
	if useStaging {
		_ = os.RemoveAll(b.Tx.StagingDir())
	}

	b.renderService.SetSink(b.Sink)
	b.assetService.SetSink(b.Sink)
	b.postService.SetSink(b.Sink)

	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("Panic during build, rolling back", "panic", r)
			b.Tx.Rollback()
			panic(r)
		} else {
			// Rollback only does something if not committed
			b.Tx.Rollback()
		}
	}()

	// Acquire build lock to prevent concurrent builds
	buildLock, lockErr := utils.AcquireBuildLock(b.cfg.OutputDir)
	if lockErr != nil {
		if !b.cfg.ForceLock {
			return fmt.Errorf("could not acquire build lock: %w (use --force-lock to override)", lockErr)
		}
		b.logger.Warn("Acquiring build lock failed, but continuing due to --force-lock", "error", lockErr)
	} else {
		defer func() { _ = buildLock.Release() }()
	}

	// Warn about empty baseURL in production mode
	if b.cfg.BaseURL == "" && !b.cfg.IsDev {
		b.logger.Warn("baseURL is empty in production mode - links will be relative and may not work correctly")
		b.logger.Info("Set baseURL in kosh.yaml or use -baseurl flag for production builds")
	}

	cfg := b.cfg
	// Build started - minimal logging

	// Clear stale sync cache
	utils.ClearSyncCache()

	// 1. Setup & Cache Invalidation
	// Use a channel to signal when templates are fully loaded.
	// This prevents rehydration from reading partially-loaded templates.
	templateReady := make(chan struct{})
	go func() {
		b.renderService.ReloadTemplates()
		close(templateReady)
	}()

	var setupWg sync.WaitGroup
	setupWg.Add(1)
	go func() {
		defer setupWg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			b.checkWasmUpdate()
		}
	}()

	globalDependencies := []string{
		filepath.Join(cfg.TemplateDir, "layout.html"),
		filepath.Join(cfg.TemplateDir, "index.html"),
		filepath.Join(cfg.TemplateDir, "404.html"),
		filepath.Join(cfg.TemplateDir, "graph.html"),
		filepath.Join(cfg.StaticDir, "css/layout.css"),
		filepath.Join(cfg.StaticDir, "css/theme.css"),
		"kosh.yaml",
		"builder/generators/pwa.go",
	}
	forceSocialRebuild := false
	shouldForce := b.cfg.ForceRebuild
	var affectedPosts []string
	var lastBuildTime time.Time

	if indexInfo, err := os.Stat(filepath.Join(b.cfg.OutputDir, "index.html")); err == nil {
		lastBuildTime = indexInfo.ModTime()

		// Parallelize dependency checks for better performance
		var depMu sync.Mutex
		var depWg sync.WaitGroup

		for _, dep := range globalDependencies {
			depWg.Add(1)
			go func(depPath string) {
				defer depWg.Done()
				if info, err := os.Stat(depPath); err == nil && info.ModTime().After(lastBuildTime) {
					affected := b.invalidateForTemplate(depPath)
					depMu.Lock()
					if affected != nil {
						affectedPosts = append(affectedPosts, affected...)
					} else {
						shouldForce = true
					}
					depMu.Unlock()
				}
			}(dep)
		}
		depWg.Wait()

		if info, err := os.Stat("builder/generators/social.go"); err == nil && info.ModTime().After(lastBuildTime) {
			forceSocialRebuild = true
		}
	}

	// Note: ForceRebuild is NOT reset here. It's reset after setup tasks complete
	// to avoid a race with async checks (WASM, template invalidation) that may set it.

	// Pre-create output directories in VFS
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.cfg.OutputDir, dir)); err != nil {
			b.logger.Error("Failed to create directory", "dir", dir, "error", err)
		}
	}

	// Wait for background setup (WASM compilation, etc.) to complete before asset building
	setupWg.Wait()

	// 2. Static Assets (MUST complete before posts to populate Assets map)
	fmt.Println("📦 Building assets...")
	assetTimer := utils.StartPhase("Asset building")
	if err := b.copyStaticAndBuildAssets(ctx); err != nil {
		assetTimer.Stop()
		return err
	}
	assetTimer.Stop()
	_ = b.Sink.WriteFile(filepath.Join(b.cfg.OutputDir, ".nojekyll"), []byte(""))

	if len(affectedPosts) > 0 && b.cacheService != nil {
		for _, postPath := range affectedPosts {
			relPath, _ := utils.SafeRel(b.cfg.ContentDir, postPath)
			// Need PostID to delete.
			// invalidateForTemplate returns paths.
			// We can generate ID from path (empty UUID).
			postID := cache.GeneratePostID("", relPath)
			_ = b.cacheService.DeletePost(postID)
		}
	}

	// 3. Process Content (Posts)
	allPosts, pinnedPosts, tagMap, indexedPosts, anyPostChanged, has404, err := b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing)
	if err != nil {
		return err
	}
	_ = anyPostChanged

	fmt.Println("   ✅ Content processed.")

	// Store indexed posts for incremental builds
	b.indexedPosts = indexedPosts

	// 4. Generate Global Pages (Parallelized)
	g, gCtx := errgroup.WithContext(ctx)

	// 4. Global Pages (Always run to ensure consistency and prevent orphan deletion)
	g.Go(func() error {
		fmt.Println("📄 Rendering pagination...")
		paginationTimer := utils.StartPhase("Pagination")
		defer paginationTimer.Stop()
		return b.renderPagination(gCtx, allPosts, pinnedPosts, shouldForce)
	})

	if !has404 {
		g.Go(func() error {
			b.renderService.Render404(filepath.Join(b.cfg.OutputDir, "404.html"), models.PageData{
				BaseURL:        cfg.BaseURL,
				BuildVersion:   cfg.BuildVersion,
				Config:         cfg,
				TabTitle:       "404 - Page Not Found | " + cfg.Title,
				RelativePrefix: "",
			})
			return nil
		})
	}

	g.Go(func() error {
		fmt.Println("🏷️  Rendering tags...")
		tagsTimer := utils.StartPhase("Tags rendering")
		defer tagsTimer.Stop()
		return b.renderTags(gCtx, tagMap, forceSocialRebuild)
	})

	g.Go(func() error {
		fmt.Println("🕸️  Rendering graph and metadata...")
		graphTimer := utils.StartPhase("Graph and metadata")
		defer graphTimer.Stop()
		b.renderService.RenderGraph(filepath.Join(b.cfg.OutputDir, "graph.html"), models.PageData{
			Title:          "Graph View",
			TabTitle:       "Knowledge Graph | " + cfg.Title,
			BaseURL:        cfg.BaseURL,
			BuildVersion:   cfg.BuildVersion,
			Config:         cfg,
			Assets:         b.renderService.GetAssets(),
			RelativePrefix: "",
		})
		allContent := make([]models.PostMetadata, 0, len(allPosts)+len(pinnedPosts))
		allContent = append(allContent, allPosts...)
		allContent = append(allContent, pinnedPosts...)
		return b.generateMetadata(gCtx, allContent, tagMap, indexedPosts, shouldForce)
	})

	// 5. PWA (Run concurrently)
	if cfg.Features.Generators.PWA {
		g.Go(func() error {
			fmt.Println("📱 Generating PWA...")
			pwaTimer := utils.StartPhase("PWA generation")
			defer pwaTimer.Stop()
			return b.generatePWA(gCtx, shouldForce)
		})
	}

	// Wait for all parallel tasks (Global pages + PWA) to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel build tasks failed: %w", err)
	}

	// Reset ForceRebuild AFTER all async checks have completed
	b.cfg.ForceRebuild = false

	fmt.Println("💾 Publishing output...")
	syncTimer := utils.StartPhase("Publish")
	if err := b.Tx.Commit(); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	// Clear memory state
	b.renderService.ClearRenderedFiles()

	// Build complete
	return nil
}

func (b *Builder) copyStaticAndBuildAssets(ctx context.Context) error {
	if err := b.assetService.Build(ctx); err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}
	return nil
}

func (b *Builder) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool) ([]models.PostMetadata, []models.PostMetadata, map[string][]models.PostMetadata, []models.IndexedPost, bool, bool, error) {
	result, err := b.postService.Process(ctx, shouldForce, forceSocialRebuild, outputMissing)
	if err != nil {
		return nil, nil, nil, nil, false, false, err
	}
	return result.AllPosts, result.PinnedPosts, result.TagMap, result.IndexedPosts, result.AnyPostChanged, result.Has404, nil
}
