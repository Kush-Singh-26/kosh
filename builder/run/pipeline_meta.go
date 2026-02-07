package run

import (
	"my-ssg/builder/generators"
	"my-ssg/builder/models"
	"my-ssg/builder/utils"
	"sync"
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
		_ = generators.GenerateSearchIndex(b.DestFs, "public", indexedPosts)
	}()
	graphHash, _ := utils.GetGraphHash(allContent)
	if shouldForce || b.socialCardCache.GraphHash != graphHash {
		genWg.Add(1)
		go func() {
			defer genWg.Done()
			generators.GenerateGraph(b.DestFs, cfg.BaseURL, allContent)
			b.mu.Lock()
			b.socialCardCache.GraphHash = graphHash
			b.mu.Unlock()
		}()
	}
	genWg.Wait()
}
