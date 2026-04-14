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
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// SiteWideOptions configures site-wide generator orchestration.
type SiteWideOptions struct {
	Ctx                context.Context
	AssetsReadySignal  <-chan struct{}
	WasmWaitGroup      *sync.WaitGroup
	ForceSocialRebuild bool
	SearchIndex        *models.SearchIndex
}

func (engineInstance *Engine) setupSiteWideRendering(options SiteWideOptions) (func(*post.ContentContext, bool) (*errgroup.Group, *timeutil.PhaseTimer), *timeutil.PhaseTimer) {
	workingContext := options.Ctx
	assetsReadySignal := options.AssetsReadySignal
	wasmWaitGroup := options.WasmWaitGroup
	forceSocialRebuild := options.ForceSocialRebuild
	psearchIndex := options.SearchIndex

	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *timeutil.PhaseTimer
	var siteWideOnce sync.Once

	runSiteWide := func(metadataContext *post.ContentContext, assetsChanged bool) (*errgroup.Group, *timeutil.PhaseTimer) {
		if engineInstance.Search != nil && metadataContext.IndexedPosts != nil {
			engineInstance.Search.SetIndexedPosts(metadataContext.IndexedPosts)
		}
		if psearchIndex != nil {
			// If we have a pipelined index, use it instead of building it from IndexedPosts
			metadataContext.PrebuiltSearchIndex = psearchIndex
		}

		if engineInstance.shouldSkipSiteWideRendering(metadataContext, assetsChanged) {
			return nil, nil
		}

		siteWideOnce.Do(func() {
			engineInstance.Deps.Logger.Info("Rendering pagination, tags, metadata and PWA...")
			siteTimer = timeutil.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(workingContext)

			siteWideGroup.Go(func() error {
				engineInstance.Assets.WaitForAvailability(siteWideCtx, assetsReadySignal)
				return engineInstance.renderPagination(renderPaginationOptions{
					workingContext: siteWideCtx,
					allPosts:       metadataContext.AllPosts,
					pinnedPosts:    metadataContext.PinnedPosts,
					force:          engineInstance.Cfg.ShouldForceRebuild,
					taxonomies:     metadataContext.Taxonomies,
				})
			})
			siteWideGroup.Go(func() error {
				engineInstance.Assets.WaitForAvailability(siteWideCtx, assetsReadySignal)
				return engineInstance.renderTaxonomies(siteWideCtx, metadataContext.TaxonomyMap, forceSocialRebuild)
			})
			siteWideGroup.Go(func() error {
				return engineInstance.renderSiteMetadata(MetadataRenderOptions{
					AllPosts:              metadataContext.AllPosts,
					TaxonomyMapSummarized: metadataContext.Taxonomies,
					AssetsReadySignal:     assetsReadySignal,
				})
			})
			wasmWaitGroup.Add(1)
			async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
				Ctx:       workingContext,
				Logger:    engineInstance.Deps.Logger,
				Operation: "pwa generation",
				Fn: func() error {
					engineInstance.Assets.WaitForAvailability(workingContext, assetsReadySignal)
					if err := engineInstance.generatePWA(workingContext, engineInstance.Cfg.ShouldForceRebuild); err != nil {
						engineInstance.Deps.Logger.Warn("PWA generation failed", "error", err)
					}
					return nil
				},
				Cleanup: wasmWaitGroup.Done,
			})
		})

		if metadataContext.IndexedPosts != nil || metadataContext.PrebuiltSearchIndex != nil {
			siteWideGroup.Go(func() error {
				return engineInstance.renderSiteMetadata(MetadataRenderOptions{
					IndexedPosts: metadataContext.IndexedPosts,
					SearchIndex:  metadataContext.PrebuiltSearchIndex,
				})
			})
		}

		return siteWideGroup, siteTimer
	}

	return runSiteWide, nil
}

func (engineInstance *Engine) shouldSkipSiteWideRendering(metadataContext *post.ContentContext, assetsChanged bool) bool {
	useStaging := !engineInstance.Cfg.IsDev || engineInstance.State.IsCleanBuild
	if metadataContext.AnyPostChanged || engineInstance.State.IsCleanBuild || useStaging || engineInstance.State.ForceGenerators.Load() || assetsChanged {
		engineInstance.State.ForceGenerators.Store(false)
		return false
	}
	return true
}

// MetadataRenderOptions configures site-wide metadata generation.
type MetadataRenderOptions struct {
	AllPosts              []models.PostMetadata
	TaxonomyMapSummarized map[string]models.TaxonomyData
	TaxonomyMap           map[string]map[string][]models.PostMetadata
	IndexedPosts          []models.IndexedPost
	SearchIndex           *models.SearchIndex
	AssetsReadySignal     <-chan struct{}
}

func (engineInstance *Engine) generateSitemap(options MetadataRenderOptions) error {
	_, err := generators.GenerateSitemap(generators.SitemapOptions{
		Sink:       engineInstance.artifactSink,
		BaseURL:    engineInstance.Cfg.BaseURL,
		Posts:      options.AllPosts,
		Taxonomies: options.TaxonomyMap,
		OutputPath: filepath.Join(engineInstance.Cfg.OutputDir, "sitemap/sitemap.xml"),
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
		Posts:       options.AllPosts,
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
		searchPath, size, err = generators.GenerateSearchIndexFromObject(engineInstance.artifactSink, options.SearchIndex)
	} else {
		searchPath, size, err = generators.GenerateSearchIndex(engineInstance.artifactSink, options.IndexedPosts)
	}

	if err != nil {
		return fmt.Errorf("failed to generate search index: %w", err)
	}

	if engineInstance.Health != nil {
		var docs int64
		if options.SearchIndex != nil {
			docs = options.SearchIndex.TotalDocs
		} else {
			docs = int64(len(options.IndexedPosts))
		}
		engineInstance.Health.RecordSearchStats(docs, size)
	}

	engineInstance.Deps.Logger.Debug("Search index generated", "path", searchPath, "size", size)
	return nil
}

func (engineInstance *Engine) generateGraph(options MetadataRenderOptions) error {
	_, _, err := generators.GenerateGraph(generators.GraphOptions{
		Sink:       engineInstance.artifactSink,
		BaseURL:    engineInstance.Cfg.BaseURL,
		Posts:      options.AllPosts,
		OutputPath: filepath.Join(engineInstance.Cfg.OutputDir, "graph.json"),
		Config:     engineInstance.Cfg.Features.Generators.Graph,
		SiteTitle:  engineInstance.Cfg.Title,
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
		IsGraphPage:    true,
		Context:        models.ContextHome,
	}); err != nil {
		return fmt.Errorf("failed to render graph page: %w", err)
	}
	return nil
}

func (engineInstance *Engine) renderSiteMetadata(options MetadataRenderOptions) error {
	errorGroup := new(errgroup.Group)

	if engineInstance.Cfg.Features.Generators.IsSitemapEnabled && options.AllPosts != nil && options.IndexedPosts == nil {
		errorGroup.Go(func() error {
			return engineInstance.generateSitemap(options)
		})
	}

	if engineInstance.Cfg.Features.Generators.IsRSSEnabled && options.AllPosts != nil && options.IndexedPosts == nil {
		errorGroup.Go(func() error {
			return engineInstance.generateRSS(options)
		})
	}

	if engineInstance.Cfg.Features.Generators.IsSearchEnabled && (options.IndexedPosts != nil || options.SearchIndex != nil) {
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

// RenderSiteWide triggers a subset of site-wide generators suitable for incremental builds.
// In dev mode, this focuses on pagination (index.html) to maintain consistency without
// the overhead of full metadata (RSS/Sitemap) or PWA regeneration.
func (engineInstance *Engine) RenderSiteWide(ctx context.Context, metadataContext *post.ContentContext) error {
	if engineInstance.shouldSkipSiteWideRendering(metadataContext, false) {
		return nil
	}

	errorGroup, siteWideCtx := errgroup.WithContext(ctx)

	// Always render pagination to ensure index.html consists of the latest post list/snippets.
	errorGroup.Go(func() error {
		return engineInstance.renderPagination(renderPaginationOptions{
			workingContext: siteWideCtx,
			allPosts:       metadataContext.AllPosts,
			pinnedPosts:    metadataContext.PinnedPosts,
			force:          false,
			taxonomies:     metadataContext.Taxonomies,
		})
	})

	// Also render site-wide metadata (graph, RSS, sitemap) during incremental builds
	// if a change was detected.
	errorGroup.Go(func() error {
		return engineInstance.renderSiteMetadata(MetadataRenderOptions{
			AllPosts:              metadataContext.AllPosts,
			TaxonomyMapSummarized: metadataContext.Taxonomies,
			TaxonomyMap:           metadataContext.TaxonomyMap,
		})
	})

	if metadataContext.IndexedPosts != nil {
		errorGroup.Go(func() error {
			return engineInstance.renderSiteMetadata(MetadataRenderOptions{
				IndexedPosts: metadataContext.IndexedPosts,
			})
		})
	}

	return errorGroup.Wait()
}
