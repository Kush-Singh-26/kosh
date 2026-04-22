package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// SiteWideOptions configures site-wide generator orchestration.
type SiteWideOptions struct {
	Ctx                context.Context
	AssetsReadySignal  <-chan struct{}
	WasmWaitGroup      *sync.WaitGroup
	ForceSocialRebuild bool
	SearchIndex        *searchpkg.SearchIndex
}

func (engineInstance *Engine) updateSearchIndex(metadataContext *content.Context, psearchIndex *searchpkg.SearchIndex) {
	if engineInstance.Search != nil && metadataContext.IndexedItems != nil {
		engineInstance.Search.SetIndexedPosts(metadataContext.IndexedItems)
	}
	if psearchIndex != nil {
		metadataContext.PrebuiltSearchIndex = psearchIndex
	}
}

func (engineInstance *Engine) submitSiteWideTasks(ctx context.Context, group *errgroup.Group, metadataContext *content.Context, assetsReadySignal <-chan struct{}, forceSocialRebuild bool, searchIndex *searchpkg.SearchIndex) {
	group.Go(func() error {
		engineInstance.Assets.WaitForAvailability(ctx, assetsReadySignal)
		return engineInstance.renderPagination(renderPaginationOptions{
			workingContext: ctx,
			allPosts:       metadataContext.AllItems,
			pinnedItems:    metadataContext.PinnedItems,
			force:          engineInstance.Cfg.ShouldForceRebuild,
			taxonomies:     metadataContext.Taxonomies,
		})
	})
	group.Go(func() error {
		engineInstance.Assets.WaitForAvailability(ctx, assetsReadySignal)
		return engineInstance.renderTaxonomies(ctx, metadataContext.TaxonomyMap, forceSocialRebuild)
	})
	group.Go(func() error {
		return engineInstance.renderSiteMetadata(MetadataRenderOptions{
			AllPosts:              metadataContext.AllItems,
			TaxonomyMapSummarized: metadataContext.Taxonomies,
			TaxonomyMap:           metadataContext.TaxonomyMap,
			AssetsReadySignal:     assetsReadySignal,
			IndexedPosts:          metadataContext.IndexedItems,
			SearchIndex:           searchIndex,
		})
	})
	group.Go(func() error {
		return engineInstance.renderDataPages(ctx)
	})
}


func (engineInstance *Engine) handlePWAGeneration(ctx context.Context, wasmWaitGroup *sync.WaitGroup, assetsReadySignal <-chan struct{}) {
	wasmWaitGroup.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    engineInstance.Deps.Logger,
		Operation: "pwa generation",
		Fn: func() error {
			engineInstance.Assets.WaitForAvailability(ctx, assetsReadySignal)
			if err := engineInstance.generatePWA(ctx, engineInstance.Cfg.ShouldForceRebuild); err != nil {
				engineInstance.Deps.Logger.Warn("PWA generation failed", "error", err)
			}
			return nil
		},
		Cleanup: wasmWaitGroup.Done,
	})
}

func (engineInstance *Engine) setupSiteWideRendering(options SiteWideOptions) (func(*content.Context, bool) (*errgroup.Group, *timeutil.PhaseTimer), *timeutil.PhaseTimer) {
	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *timeutil.PhaseTimer
	var siteWideOnce sync.Once

	runSiteWide := func(metadataContext *content.Context, assetsChanged bool) (*errgroup.Group, *timeutil.PhaseTimer) {
		engineInstance.updateSearchIndex(metadataContext, options.SearchIndex)

		if engineInstance.shouldSkipSiteWideRendering(metadataContext, assetsChanged) {
			return nil, nil
		}

		siteWideOnce.Do(func() {
			engineInstance.Deps.Logger.Info("Rendering pagination, taxonomies, metadata and PWA...")
			siteTimer = timeutil.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(options.Ctx)

			engineInstance.submitSiteWideTasks(siteWideCtx, siteWideGroup, metadataContext, options.AssetsReadySignal, options.ForceSocialRebuild, options.SearchIndex)
			engineInstance.handlePWAGeneration(options.Ctx, options.WasmWaitGroup, options.AssetsReadySignal)
		})

		return siteWideGroup, siteTimer
	}

	return runSiteWide, nil
}


func (engineInstance *Engine) shouldSkipSiteWideRendering(metadataContext *content.Context, assetsChanged bool) bool {
	useStaging := !engineInstance.Cfg.IsDev || engineInstance.State.IsCleanBuild
	if metadataContext.AnyItemChanged || engineInstance.State.IsCleanBuild || useStaging || engineInstance.State.ForceGenerators.Load() || assetsChanged {
		engineInstance.State.ForceGenerators.Store(false)
		return false
	}
	return true
}

// MetadataRenderOptions configures site-wide metadata generation.
type MetadataRenderOptions struct {
	AllPosts              []models.ContentMetadata
	TaxonomyMapSummarized map[string]models.TaxonomyData
	TaxonomyMap           map[string]map[string][]models.ContentMetadata
	IndexedPosts          []searchpkg.IndexedContent
	SearchIndex           *searchpkg.SearchIndex
	AssetsReadySignal     <-chan struct{}
}

func (engineInstance *Engine) generateSitemap(options MetadataRenderOptions) error {
	_, err := generators.GenerateSitemap(generators.SitemapOptions{
		Sink:          engineInstance.artifactSink,
		BaseURL:       engineInstance.Cfg.BaseURL,
		Items:         options.AllPosts,
		Taxonomies:    options.TaxonomyMap,
		ContentPrefix: engineInstance.Cfg.ContentPrefix,
		OutputPath:    filepath.Join(engineInstance.Cfg.OutputDir, "sitemap/sitemap.xml"),
	})
	if err != nil {
		engineInstance.Deps.Logger.Error("Failed to generate sitemap", "error", err)
		return err
	}
	engineInstance.Deps.Render.RegisterFile(filepath.Join(engineInstance.Cfg.OutputDir, "sitemap/sitemap.xml"))
	return nil
}

func (engineInstance *Engine) generateRSS(options MetadataRenderOptions) error {
	_, err := generators.GenerateRSS(generators.RSSOptions{
		Sink:        engineInstance.artifactSink,
		BaseURL:     engineInstance.Cfg.BaseURL,
		Items:       options.AllPosts,
		Title:       engineInstance.Cfg.Title,
		Description: engineInstance.Cfg.Description,
		Author:      engineInstance.Cfg.Author.Name,
		LogoURL:     engineInstance.Cfg.BaseURL + engineInstance.Cfg.Logo,
		OutputPath:  filepath.Join(engineInstance.Cfg.OutputDir, "rss.xml"),
	})
	if err != nil {
		engineInstance.Deps.Logger.Error("Failed to generate RSS feed", "error", err)
		return err
	}
	engineInstance.Deps.Render.RegisterFile(filepath.Join(engineInstance.Cfg.OutputDir, "rss.xml"))
	return nil
}

func (engineInstance *Engine) generateSearchIndex(options MetadataRenderOptions) error {
	var searchPath string
	var size int64
	var err error

	if options.SearchIndex != nil {
		options.SearchIndex.Ranking = engineInstance.Cfg.Features.Generators.Search.Ranking
		searchPath, size, err = generators.GenerateSearchIndexFromObject(engineInstance.artifactSink, options.SearchIndex)
	} else {
		searchPath, size, err = generators.GenerateSearchIndex(engineInstance.artifactSink, options.IndexedPosts, engineInstance.Cfg.Features.Generators.Search.Ranking)
	}

	if err != nil {
		return fmt.Errorf("failed to generate search index: %w", err)
	}

	if engineInstance.Health != nil {
		var docs int64
		if options.SearchIndex != nil {
			docs = options.SearchIndex.TotalItems
		} else {
			docs = int64(len(options.IndexedPosts))
		}
		isEnabled := engineInstance.Cfg.Features.Generators.Search.IsEnabled
		// Search is always "in sync" if regenerated successfully here
		engineInstance.Health.RecordSearchStats(docs, size, isEnabled, true)
	}

	engineInstance.Deps.Logger.Debug("Search index generated", "path", searchPath, "size", size)
	return nil
}

func (engineInstance *Engine) generateGraph(options MetadataRenderOptions) error {
	_, _, err := generators.GenerateGraph(generators.GraphOptions{
		Sink:          engineInstance.artifactSink,
		BaseURL:       engineInstance.Cfg.BaseURL,
		Items:         options.AllPosts,
		OutputPath:    filepath.Join(engineInstance.Cfg.OutputDir, "graph.json"),
		Config:        engineInstance.Cfg.Features.Generators.Graph,
		SiteTitle:     engineInstance.Cfg.Title,
		ContentPrefix: engineInstance.Cfg.ContentPrefix,
	})
	if err != nil {
		engineInstance.Deps.Logger.Error("Failed to generate knowledge graph data", "error", err)
		return err
	}
	engineInstance.Deps.Render.RegisterFile(filepath.Join(engineInstance.Cfg.OutputDir, "graph.json"))

	if options.AssetsReadySignal != nil {
		<-options.AssetsReadySignal
	}
	if err := engineInstance.Deps.Render.RenderGraph(filepath.Join(engineInstance.Cfg.OutputDir, "graph.html"), models.PageData{
		Title:          "Graph View",
		TabTitle:       "Knowledge Graph | " + engineInstance.Cfg.Title,
		BaseURL:        engineInstance.Cfg.BaseURL,
		BuildVersion:   engineInstance.Cfg.BuildVersion,
		Config:         engineInstance.Cfg,
		Taxonomies:     options.TaxonomyMapSummarized, // Use summarized for template
		RelativePrefix: "",
		ContentPrefix:  engineInstance.Cfg.ContentPrefix,
		IsGraphPage:    true,
		Context:        models.ContextHome,
	}); err != nil {
		return fmt.Errorf("failed to render graph page: %w", err)
	}
	return nil
}

func (engineInstance *Engine) renderSiteMetadata(options MetadataRenderOptions) error {
	errorGroup := new(errgroup.Group)

	if engineInstance.Cfg.Features.Generators.IsSitemapEnabled && options.AllPosts != nil {
		errorGroup.Go(func() error {
			return engineInstance.generateSitemap(options)
		})
	}

	if engineInstance.Cfg.Features.Generators.IsRSSEnabled && options.AllPosts != nil {
		errorGroup.Go(func() error {
			return engineInstance.generateRSS(options)
		})
	}

	if engineInstance.Cfg.Features.Generators.Search.IsEnabled && (options.IndexedPosts != nil || options.SearchIndex != nil) {
		errorGroup.Go(func() error {
			return engineInstance.generateSearchIndex(options)
		})
	}

	if engineInstance.Cfg.Features.Generators.Graph.IsEnabled && len(options.AllPosts) > 0 {
		errorGroup.Go(func() error {
			return engineInstance.generateGraph(options)
		})
	}

	return errorGroup.Wait()
}

// RenderSiteWide triggers site-wide generators for incremental builds.
// Runs pagination, taxonomies, sitemap/RSS/graph/search and data pages.
func (engineInstance *Engine) RenderSiteWide(ctx context.Context, metadataContext *content.Context) error {
	if engineInstance.shouldSkipSiteWideRendering(metadataContext, false) {
		return nil
	}

	errorGroup, siteWideCtx := errgroup.WithContext(ctx)

	// Pagination — always update index.html with the latest content list.
	errorGroup.Go(func() error {
		return engineInstance.renderPagination(renderPaginationOptions{
			workingContext: siteWideCtx,
			allPosts:       metadataContext.AllItems,
			pinnedItems:    metadataContext.PinnedItems,
			force:          false,
			taxonomies:     metadataContext.Taxonomies,
		})
	})

	// Taxonomy pages — update tag/category indices on content change.
	errorGroup.Go(func() error {
		return engineInstance.renderTaxonomies(siteWideCtx, metadataContext.TaxonomyMap, false)
	})

	// Metadata (RSS, sitemap, graph) + search index in one consolidated call.
	errorGroup.Go(func() error {
		return engineInstance.renderSiteMetadata(MetadataRenderOptions{
			AllPosts:              metadataContext.AllItems,
			TaxonomyMapSummarized: metadataContext.Taxonomies,
			TaxonomyMap:           metadataContext.TaxonomyMap,
			IndexedPosts:          metadataContext.IndexedItems,
			SearchIndex:           metadataContext.PrebuiltSearchIndex,
		})
	})

	return errorGroup.Wait()
}


func (engineInstance *Engine) renderDataPages(ctx context.Context) error {
	return generators.RenderDataPages(generators.DataPagesOptions{
		Ctx:    ctx,
		Cfg:    engineInstance.Cfg,
		Render: engineInstance.Deps.Render,
		Data:   engineInstance.Cfg.SiteData,
	})
}
