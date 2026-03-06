package run

import (
	"context"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (b *Builder) generateMetadata(ctx context.Context, allContent []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, shouldForce bool) error {
	cfg := b.cfg
	outputDir := cfg.OutputDir

	g, _ := errgroup.WithContext(ctx)

	if cfg.Features.Generators.Sitemap {
		g.Go(func() error {
			path, err := generators.GenerateSitemap(b.Sink, cfg.BaseURL, allContent, tagMap, filepath.Join(outputDir, "sitemap", "sitemap.xml"))
			if err == nil {
				b.renderService.RegisterFile(path)
			}
			return err
		})
	}

	if cfg.Features.Generators.RSS {
		g.Go(func() error {
			path, err := generators.GenerateRSS(b.Sink, cfg.BaseURL, allContent, cfg.Title, cfg.Description, filepath.Join(outputDir, "rss.xml"))
			if err == nil {
				b.renderService.RegisterFile(path)
			}
			return err
		})
	}

	if cfg.Features.Generators.Search {
		g.Go(func() error {
			searchTimer := utils.StartPhase("Search index generation")
			defer searchTimer.Stop()
			path, err := generators.GenerateSearchIndex(b.Sink, outputDir, indexedPosts)
			if err == nil {
				b.renderService.RegisterFile(path)
			}
			return err
		})
	}

	if cfg.Features.Generators.Graph {
		graphHash, _ := utils.GetGraphHash(allContent)
		cachedGraphHash := ""
		if b.cacheService != nil {
			cachedGraphHash, _ = b.cacheService.GetGraphHash()
		}

		// Check if graph.json exists on disk
		graphExists := false
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, "graph.json")); err == nil {
			graphExists = true
		}

		if shouldForce || !graphExists || cachedGraphHash != graphHash {
			g.Go(func() error {
				path, err := generators.GenerateGraph(b.Sink, cfg.BaseURL, allContent, filepath.Join(outputDir, "graph.json"))
				if err == nil {
					b.renderService.RegisterFile(path)
					if b.cacheService != nil {
						_ = b.cacheService.SetGraphHash(graphHash)
					}
				}
				return err
			})
		}
	}
	return g.Wait()
}
