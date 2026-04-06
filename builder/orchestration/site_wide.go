package orchestration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	"golang.org/x/sync/errgroup"
)

func (b *Engine) setupSiteWideRendering(
	ctx context.Context,
	assetsReady <-chan struct{},
	wasmWg *sync.WaitGroup,
	forceSocialRebuild bool,
) (func(*post.MetadataContext, bool) (*errgroup.Group, *timeutil.PhaseTimer), *timeutil.PhaseTimer) {
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
					b.Deps.Logger.Warn("PWA generation failed", "error", err)
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

func (b *Engine) shouldSkipSiteWideRendering(cb *post.MetadataContext, assetsChanged bool) bool {
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

	if b.Cfg.Features.Generators.RSS && allPosts != nil && indexedPosts == nil {
		g.Go(func() error {
			_, err := generators.GenerateRSS(b.Sink, b.Cfg.BaseURL, allPosts, b.Cfg.Title, b.Cfg.Description, filepath.Join(b.Cfg.OutputDir, "rss.xml"))
			if err == nil {
				b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, "rss.xml"))
			} else {
				b.Deps.Logger.Error("Failed to generate RSS feed", "error", err)
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
				b.Deps.Logger.Error("Failed to generate search index", "error", err)
				return err
			}
			return nil
		})
	}

	if b.Cfg.Features.Generators.Graph.Enabled && len(allPosts) > 0 {
		g.Go(func() error {
			currentHash, err := generators.ComputeGraphHash(allPosts)
			if err != nil {
				b.Deps.Logger.Error("Failed to compute graph hash", "error", err)
				return err
			}

			// In dev mode, always regenerate to avoid orphan cleanup deleting the file.
			// In production/clean builds, skip if unchanged to save I/O during staging.
			graphDataChanged := b.Cfg.IsDev
			if !graphDataChanged && b.Deps.Cache != nil {
				cachedHash, cacheErr := b.Deps.Cache.GetGraphHash()
				if cacheErr == nil && cachedHash == currentHash {
					b.Deps.Logger.Info("Graph data unchanged, skipping JSON regeneration")
					graphDataChanged = false
				} else {
					graphDataChanged = true
				}
			}
			// If cache is nil (cold build), always regenerate
			if !graphDataChanged && b.Deps.Cache == nil {
				graphDataChanged = true
			}

			if graphDataChanged {
				_, _, err = generators.GenerateGraph(b.Sink, b.Cfg.BaseURL, allPosts, filepath.Join(b.Cfg.OutputDir, "graph.json"), b.Cfg.Features.Generators.Graph, b.Cfg.Title)
				if err != nil {
					b.Deps.Logger.Error("Failed to generate knowledge graph data", "error", err)
					return err
				}

				if b.Deps.Cache != nil {
					if err := b.Deps.Cache.SetGraphHash(currentHash); err != nil {
						b.Deps.Logger.Warn("Failed to cache graph hash", "error", err)
					}
				}
			}

			if assetsReady != nil {
				<-assetsReady
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
