package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	"github.com/spf13/afero"
)

const (
	pwaCacheDirMode  = 0755
	pwaCacheFileMode = 0644
	pwaIconSmall     = 192
	pwaIconLarge     = 512
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shouldGeneratePWAIcons(shouldForce, hasHashFile, hasCache192, hasCache512 bool) bool {
	return shouldForce || !hasHashFile || !hasCache192 || !hasCache512
}

type renderRegistrar interface {
	RegisterFile(string)
}

func (b *Engine) generateServiceWorker(shouldForce bool) error {
	return generators.GenerateSW(generators.SWOptions{
		Sink:         b.Sink,
		DestDir:      b.Cfg.OutputDir,
		BuildVersion: b.Cfg.BuildVersion,
		ForceRebuild: shouldForce,
		BaseURL:      b.Cfg.BaseURL,
		Assets:       b.Deps.Render.GetAssets(),
		IsTesting:    b.Ctx.IsTesting,
	})
}

func (b *Engine) generateManifest(shouldForce bool) error {
	return generators.GenerateManifest(generators.ManifestOptions{
		Sink:            b.Sink,
		DestDir:         b.Cfg.OutputDir,
		BaseURL:         b.Cfg.BaseURL,
		SiteTitle:       b.Cfg.Title,
		SiteDescription: b.Cfg.Description,
		ForceRebuild:    shouldForce,
		IsTesting:       b.Ctx.IsTesting,
	})
}

func pwaCachePaths(cacheDir, hash string) (string, string, string) {
	cacheHashFile := filepath.Join(cacheDir, hash+".hash")
	cache192 := filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconSmall))
	cache512 := filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconLarge))
	return cacheHashFile, cache192, cache512
}

func registerPWAIcons(render renderRegistrar, outputDir string) {
	render.RegisterFile(filepath.Join(outputDir, "static/images/icon-192.png"))
	render.RegisterFile(filepath.Join(outputDir, "static/images/icon-512.png"))
}

func (b *Engine) writeGeneratedPWAIcons(cacheDir, hash string, icons map[int][]byte) {
	_ = os.MkdirAll(cacheDir, pwaCacheDirMode)
	_ = os.WriteFile(filepath.Join(cacheDir, hash+".hash"), []byte(hash), pwaCacheFileMode)

	if data, ok := icons[pwaIconSmall]; ok {
		_ = os.WriteFile(filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconSmall)), data, pwaCacheFileMode)
	}
	if data, ok := icons[pwaIconLarge]; ok {
		_ = os.WriteFile(filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconLarge)), data, pwaCacheFileMode)
	}
}

func (b *Engine) copyCachedPWAIcons(cache192, cache512 string) {
	if data, err := os.ReadFile(cache192); err == nil {
		iconPath := filepath.Join(b.Cfg.OutputDir, "static/images/icon-192.png")
		_ = b.Sink.WriteFile(iconPath, data)
		b.Deps.Render.RegisterFile(iconPath)
	}
	if data, err := os.ReadFile(cache512); err == nil {
		iconPath := filepath.Join(b.Cfg.OutputDir, "static/images/icon-512.png")
		_ = b.Sink.WriteFile(iconPath, data)
		b.Deps.Render.RegisterFile(iconPath)
	}
}

func (b *Engine) generatePWAIcons(shouldForce bool) error {
	iconTimer := timeutil.StartPhase("PWA icons")
	defer iconTimer.Stop()

	logoPath := b.getLogoPath()
	if logoPath == "" {
		return nil
	}

	exists, _ := afero.Exists(b.Deps.SourceFs, logoPath)
	if !exists {
		return nil
	}
	srcInfo, _ := b.Deps.SourceFs.Stat(logoPath)

	hashContent := fmt.Sprintf("%s-%d-%d", logoPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
	currentHash := cache.HashString(hashContent)

	cacheDir := filepath.Join(b.Cfg.CacheDir, "pwa-icons")
	cacheHashFile, cache192, cache512 := pwaCachePaths(cacheDir, currentHash)
	hasHashFile := fileExists(cacheHashFile)
	hasCache192 := fileExists(cache192)
	hasCache512 := fileExists(cache512)
	needsGeneration := shouldGeneratePWAIcons(shouldForce, hasHashFile, hasCache192, hasCache512)

	if needsGeneration {
		icons, err := generators.GeneratePWAIconBytes(b.Deps.SourceFs, logoPath, b.Deps.Logger)
		if err != nil {
			return nil
		}
		if wErr := generators.WritePWAIcons(b.Sink, filepath.Join(b.Cfg.OutputDir, "static/images"), icons); wErr == nil {
			registerPWAIcons(b.Deps.Render, b.Cfg.OutputDir)
		}
		b.writeGeneratedPWAIcons(cacheDir, currentHash, icons)
		return nil
	}

	b.copyCachedPWAIcons(cache192, cache512)
	return nil
}

func (b *Engine) generatePWA(ctx context.Context, shouldForce bool) error {
	if b.Cfg.IsDev {
		return nil
	}

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		return b.generateServiceWorker(shouldForce)
	})

	g.Go(func() error {
		return b.generateManifest(shouldForce)
	})

	g.Go(func() error {
		return b.generatePWAIcons(shouldForce)
	})

	return g.Wait()
}
