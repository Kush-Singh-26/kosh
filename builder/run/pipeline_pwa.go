package run

import (
	"my-ssg/builder/generators"
	"sync"
)

func (b *Builder) generatePWA(shouldForce bool) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if b.cfg.IsDev {
			return
		}
		_ = generators.GenerateSW(b.DestFs, "public", b.cfg.BuildVersion, shouldForce, b.cfg.BaseURL, b.rnd.Assets)
	}()
	go func() {
		defer wg.Done()
		_ = generators.GenerateManifest(b.DestFs, "public", b.cfg.BaseURL, b.cfg.Title, b.cfg.Description, shouldForce)
	}()
	go func() {
		defer wg.Done()
		_ = generators.GeneratePWAIcons(b.SourceFs, b.DestFs, "static/images/favicon.png", "public/static/images")
	}()
	wg.Wait()
}
