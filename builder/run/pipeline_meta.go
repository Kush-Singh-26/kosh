package run

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (b *Builder) generateMetadata(allContent []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, shouldForce bool) {
	cfg := b.cfg
	var genWg sync.WaitGroup
	outputDir := cfg.OutputDir

	if cfg.Features.Generators.Sitemap {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			generators.GenerateSitemap(b.DestFs, cfg.BaseURL, allContent, tagMap, filepath.Join(outputDir, "sitemap", "sitemap.xml"))
		}()
	}

	if cfg.Features.Generators.RSS {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			generators.GenerateRSS(b.DestFs, cfg.BaseURL, allContent, cfg.Title, cfg.Description, filepath.Join(outputDir, "rss.xml"))
		}()
	}

	if cfg.Features.Generators.Search {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			if err := generators.GenerateSearchIndex(b.DestFs, outputDir, indexedPosts); err != nil {
				b.logger.Error("Failed to generate search index", "error", err)
			}
		}()
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
			genWg.Add(1)
			go func() {
				defer genWg.Done()
				generators.GenerateGraph(b.DestFs, cfg.BaseURL, allContent, filepath.Join(outputDir, "graph.json"))
				if b.cacheService != nil {
					_ = b.cacheService.SetGraphHash(graphHash)
				}
			}()
		}
	}
	genWg.Wait()
}
