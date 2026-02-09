package run

import (
	"fmt"
	"os"
	"sync"

	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

func (b *Builder) generateMetadata(allContent []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, shouldForce bool) {
	cfg := b.cfg
	var genWg sync.WaitGroup
	genWg.Add(1)
	go func() {
		defer genWg.Done()
		generators.GenerateSitemap(b.DestFs, cfg.BaseURL, allContent, tagMap)
	}()
	genWg.Add(1)
	go func() {
		defer genWg.Done()
		generators.GenerateRSS(b.DestFs, cfg.BaseURL, allContent, cfg.Title, cfg.Description)
	}()
	genWg.Add(1)
	go func() {
		defer genWg.Done()
		if err := generators.GenerateSearchIndex(b.DestFs, "public", indexedPosts); err != nil {
			fmt.Printf("❌ Failed to generate search index: %v\n", err)
		}
	}()

	graphHash, _ := utils.GetGraphHash(allContent)
	cachedGraphHash := ""
	if b.cacheManager != nil {
		cachedGraphHash, _ = b.cacheManager.GetGraphHash()
	}

	// Check if graph.json exists on disk
	graphExists := false
	if _, err := os.Stat("public/graph.json"); err == nil {
		graphExists = true
	}

	if shouldForce || !graphExists || cachedGraphHash != graphHash {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			generators.GenerateGraph(b.DestFs, cfg.BaseURL, allContent)
			if b.cacheManager != nil {
				_ = b.cacheManager.SetGraphHash(graphHash)
			}
		}()
	}
	genWg.Wait()
}
