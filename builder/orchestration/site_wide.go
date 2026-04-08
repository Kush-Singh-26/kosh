package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// SiteWideOptions configures site-wide generator orchestration.
type SiteWideOptions struct {
	Ctx                context.Context
	AssetsReady        <-chan struct{}
	WasmWg             *sync.WaitGroup
	ForceSocialRebuild bool
}

func (b *Engine) setupSiteWideRendering(opts SiteWideOptions) (func(*post.MetadataContext, bool) (*errgroup.Group, *timeutil.PhaseTimer), *timeutil.PhaseTimer) {
	ctx := opts.Ctx
	assetsReady := opts.AssetsReady
	wasmWg := opts.WasmWg
	forceSocialRebuild := opts.ForceSocialRebuild

	var siteWideGroup *errgroup.Group
	var siteWideCtx context.Context
	var siteTimer *timeutil.PhaseTimer
	var siteWideOnce sync.Once

	runSiteWide := func(cb *post.MetadataContext, assetsChanged bool) (*errgroup.Group, *timeutil.PhaseTimer) {
		if b.Search != nil && cb.IndexedPosts != nil {
			b.Search.SetIndexedPosts(cb.IndexedPosts)
		}

		if b.shouldSkipSiteWideRendering(cb, assetsChanged) {
			return nil, nil
		}

		siteWideOnce.Do(func() {
			b.Deps.Logger.Info("Rendering pagination, tags, metadata and PWA...")
			siteTimer = timeutil.StartPhase("Site-wide rendering")
			siteWideGroup, siteWideCtx = errgroup.WithContext(ctx)

			var allTags []models.TagData
			for t, posts := range cb.TagMap {
				slug := timeutil.Slugify(t)
				allTags = append(allTags, models.TagData{Name: t, Count: len(posts), Link: fmt.Sprintf("/tags/%s.html", slug)})
			}

			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderPagination(renderPaginationOptions{
					ctx:         siteWideCtx,
					allPosts:    cb.AllPosts,
					pinnedPosts: cb.PinnedPosts,
					force:       b.Cfg.ForceRebuild,
					allTags:     allTags,
				})
			})
			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderTags(siteWideCtx, cb.TagMap, forceSocialRebuild)
			})
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(MetadataRenderOptions{
					AllPosts:    cb.AllPosts,
					TagMap:      cb.TagMap,
					AssetsReady: assetsReady,
				})
			})
			wasmWg.Add(1)
			go func() {
				defer wasmWg.Done()
				b.Assets.WaitForAvailability(ctx, assetsReady)
				if err := b.generatePWA(ctx, b.Cfg.ForceRebuild); err != nil {
					b.Deps.Logger.Warn("PWA generation failed", "error", err)
				}
			}()
		})

		if cb.IndexedPosts != nil {
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(MetadataRenderOptions{
					IndexedPosts: cb.IndexedPosts,
				})
			})
		}

		return siteWideGroup, siteTimer
	}

	return runSiteWide, nil
}

func (b *Engine) shouldSkipSiteWideRendering(cb *post.MetadataContext, assetsChanged bool) bool {
	useStaging := !b.Cfg.IsDev || b.State.IsCleanBuild
	if cb.AnyPostChanged || b.State.IsCleanBuild || useStaging || b.State.ForceGenerators.Load() || assetsChanged {
		b.State.ForceGenerators.Store(false)
		return false
	}
	return true
}

// MetadataRenderOptions configures site-wide metadata generation.
type MetadataRenderOptions struct {
	AllPosts     []models.PostMetadata
	TagMap       map[string][]models.PostMetadata
	IndexedPosts []models.IndexedPost
	AssetsReady  <-chan struct{}
}

func (b *Engine) renderSiteMetadata(opts MetadataRenderOptions) error {
	g := new(errgroup.Group)

	if b.Cfg.Features.Generators.Sitemap && opts.AllPosts != nil && opts.IndexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateSitemap(generators.SitemapOptions{
				Sink:       b.Sink,
				BaseURL:    b.Cfg.BaseURL,
				Posts:      opts.AllPosts,
				Tags:       opts.TagMap,
				OutputPath: filepath.Join(b.Cfg.OutputDir, "sitemap/sitemap.xml"),
			})
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "sitemap/sitemap.xml"))
			} else {
				b.Deps.Logger.Error("Failed to generate sitemap", "error", err)
				return err
			}
			return nil
		})

		g.Go(func() error {
			_, err := generators.GenerateRobotsTxt(b.Sink, b.Cfg.BaseURL, filepath.Join(b.Cfg.OutputDir, "robots.txt"))
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "robots.txt"))
			} else {
				b.Deps.Logger.Error("Failed to generate robots.txt", "error", err)
				return err
			}
			return nil
		})
	}

	if b.Cfg.Features.Generators.RSS && opts.AllPosts != nil && opts.IndexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateRSS(generators.RSSOptions{
				Sink:        b.Sink,
				BaseURL:     b.Cfg.BaseURL,
				Posts:       opts.AllPosts,
				Title:       b.Cfg.Title,
				Description: b.Cfg.Description,
				OutputPath:  filepath.Join(b.Cfg.OutputDir, "rss.xml"),
			})
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "rss.xml"))
			} else {
				b.Deps.Logger.Error("Failed to generate RSS feed", "error", err)
				return err
			}
			return nil
		})
	}

	if b.Cfg.Features.Generators.Search && opts.IndexedPosts != nil {
		g.Go(func() error {
			searchPath, size, err := generators.GenerateSearchIndex(b.Sink, opts.IndexedPosts)
			if err == nil {
				b.Deps.Render.RegisterFile(searchPath)
				if b.Health != nil {
					b.Health.RecordSearchStats(int64(len(opts.IndexedPosts)), size)
				}
			} else {
				b.Deps.Logger.Error("Failed to generate search index", "error", err)
				return err
			}
			return nil
		})
	}

	if b.Cfg.Features.Generators.Graph.Enabled && len(opts.AllPosts) > 0 {
		g.Go(func() error {
			// [omitted comment for brevity]
			_, _, err := generators.GenerateGraph(generators.GraphOptions{
				Sink:       b.Sink,
				BaseURL:    b.Cfg.BaseURL,
				Posts:      opts.AllPosts,
				OutputPath: filepath.Join(b.Cfg.OutputDir, "graph.json"),
				Config:     b.Cfg.Features.Generators.Graph,
				SiteTitle:  b.Cfg.Title,
			})
			if err != nil {
				b.Deps.Logger.Error("Failed to generate knowledge graph data", "error", err)
				return err
			}
			b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "graph.json"))

			if opts.AssetsReady != nil {
				<-opts.AssetsReady
			}
			if err := b.Deps.Render.RenderGraph(filepath.Join(b.Cfg.OutputDir, "graph.html"), models.PageData{
				Title:          "Graph View",
				TabTitle:       "Knowledge Graph | " + b.Cfg.Title,
				BaseURL:        b.Cfg.BaseURL,
				BuildVersion:   b.Cfg.BuildVersion,
				Config:         b.Cfg,
				RelativePrefix: "",
				IsGraphPage:    true,
			}); err != nil {
				return fmt.Errorf("failed to render graph page: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}
