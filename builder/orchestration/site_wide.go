package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	"golang.org/x/sync/errgroup"
)

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

	runSiteWide := func(cb *services.MetadataContext, assetsChanged bool) (*errgroup.Group, *timeutil.PhaseTimer) {
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

			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderPagination(siteWideCtx, cb.AllPosts, cb.PinnedPosts, b.Cfg.ForceRebuild)
			})
			siteWideGroup.Go(func() error {
				b.Assets.WaitForAvailability(siteWideCtx, assetsReady)
				return b.renderTags(siteWideCtx, cb.TagMap, forceSocialRebuild)
			})
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(cb.AllPosts, cb.TagMap, nil, assetsReady)
			})
			wasmWg.Add(1)
			go func() {
				defer wasmWg.Done()
				b.Assets.WaitForAvailability(ctx, assetsReady)
				if err := b.generatePWA(ctx, b.Cfg.ForceRebuild); err != nil {
					b.Logger.Warn("PWA generation failed", "error", err)
				}
			}()
		})

		if cb.IndexedPosts != nil {
			siteWideGroup.Go(func() error {
				return b.renderSiteMetadata(nil, nil, cb.IndexedPosts, nil)
			})
		}

		return siteWideGroup, siteTimer
	}

	return runSiteWide, nil
}

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

	if b.Cfg.Features.Generators.Search && indexedPosts != nil {
		g.Go(func() error {
			searchPath, size, err := generators.GenerateSearchIndex(b.Sink, indexedPosts)
			if err == nil {
				b.Deps.Render.RegisterFile(searchPath)
				if b.Health != nil {
					b.Health.RecordSearchStats(int64(len(indexedPosts)), size)
				}
			} else {
				b.Logger.Error("Failed to generate search index", "error", err)
				return err
			}
			return nil
		})
	}

	if b.Cfg.Features.Generators.Graph && len(allPosts) > 0 {
		g.Go(func() error {
			_, err := generators.GenerateGraph(b.Sink, b.Cfg.BaseURL, allPosts, filepath.Join(b.Cfg.OutputDir, "graph.json"))
			if err != nil {
				b.Logger.Error("Failed to generate knowledge graph data", "error", err)
			}

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
