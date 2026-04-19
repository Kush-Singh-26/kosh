package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
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

func (engineInstance *Engine) generateServiceWorker(shouldForce bool) error {
	return generators.GenerateSW(generators.SWOptions{
		Sink:               engineInstance.artifactSink,
		DestDir:            engineInstance.Cfg.OutputDir,
		BuildVersion:       engineInstance.Cfg.BuildVersion,
		ShouldForceRebuild: shouldForce,
		BaseURL:            engineInstance.Cfg.BaseURL,
		Assets:             engineInstance.Deps.Render.GetAssets(),
		IsTesting:          engineInstance.Ctx.IsTesting,
	})
}

func (engineInstance *Engine) generateManifest(shouldForce bool) error {
	return generators.GenerateManifest(generators.ManifestOptions{
		Sink:               engineInstance.artifactSink,
		DestDir:            engineInstance.Cfg.OutputDir,
		BaseURL:            engineInstance.Cfg.BaseURL,
		SiteTitle:          engineInstance.Cfg.Title,
		SiteDescription:    engineInstance.Cfg.Description,
		BackgroundColor:    engineInstance.Cfg.SocialCards.Background,
		ThemeColor:         engineInstance.Cfg.SocialCards.Background, // Use same background for theme color for consistency
		Icon192:            engineInstance.Cfg.Icon192,
		Icon512:            engineInstance.Cfg.Icon512,
		ShouldForceRebuild: shouldForce,
		IsTesting:          engineInstance.Ctx.IsTesting,
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

func (engineInstance *Engine) writeGeneratedPWAIcons(cacheDir, hash string, icons map[int][]byte) {
	_ = os.MkdirAll(cacheDir, pwaCacheDirMode)
	_ = os.WriteFile(filepath.Join(cacheDir, hash+".hash"), []byte(hash), pwaCacheFileMode)

	if data, ok := icons[pwaIconSmall]; ok {
		_ = os.WriteFile(filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconSmall)), data, pwaCacheFileMode)
	}
	if data, ok := icons[pwaIconLarge]; ok {
		_ = os.WriteFile(filepath.Join(cacheDir, fmt.Sprintf("%s-%d.png", hash, pwaIconLarge)), data, pwaCacheFileMode)
	}
}

func (engineInstance *Engine) copyCachedPWAIcons(cache192, cache512 string) error {
	var errs []error
	if data, err := os.ReadFile(cache192); err == nil {
		iconPath := filepath.Join(engineInstance.Cfg.OutputDir, "static/images/icon-192.png")
		_ = engineInstance.artifactSink.WriteFile(iconPath, data)
		engineInstance.Deps.Render.RegisterFile(iconPath)
	} else if !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("failed to read cached PWA icon 192: %w", err))
	}
	if data, err := os.ReadFile(cache512); err == nil {
		iconPath := filepath.Join(engineInstance.Cfg.OutputDir, "static/images/icon-512.png")
		_ = engineInstance.artifactSink.WriteFile(iconPath, data)
		engineInstance.Deps.Render.RegisterFile(iconPath)
	} else if !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("failed to read cached PWA icon 512: %w", err))
	}

	if len(errs) > 0 {
		for _, e := range errs {
			engineInstance.Deps.Logger.Warn("PWA icon cache issue", "error", e)
		}
	}
	return nil // Non-critical, continue build
}

func (engineInstance *Engine) generatePWAIcons(shouldForce bool) error {
	iconTimer := timeutil.StartPhase("PWA icons")
	defer iconTimer.Stop()

	logoPath := engineInstance.GetLogoPath()
	if logoPath == "" {
		return nil
	}

	exists, _ := afero.Exists(engineInstance.Deps.SourceFs, logoPath)
	if !exists {
		return nil
	}
	sourceFileInfo, _ := engineInstance.Deps.SourceFs.Stat(logoPath)

	hashContent := fmt.Sprintf("%s-%d-%d", logoPath, sourceFileInfo.Size(), sourceFileInfo.ModTime().UnixNano())
	currentHash := core.HashString(hashContent)

	cacheDir := filepath.Join(engineInstance.Cfg.CacheDir, "pwa-icons")
	cacheHashFile, cache192, cache512 := pwaCachePaths(cacheDir, currentHash)
	hasHashFile := fileExists(cacheHashFile)
	hasCache192 := fileExists(cache192)
	hasCache512 := fileExists(cache512)
	needsGeneration := shouldGeneratePWAIcons(shouldForce, hasHashFile, hasCache192, hasCache512)

	if needsGeneration {
		icons, err := generators.GeneratePWAIconBytes(engineInstance.Deps.SourceFs, logoPath, engineInstance.Deps.Logger)
		if err != nil {
			engineInstance.Deps.Logger.Error("Failed to generate PWA icons", "error", err)
			return err
		}
		if writeError := generators.WritePWAIcons(engineInstance.artifactSink, filepath.Join(engineInstance.Cfg.OutputDir, "static/images"), icons); writeError == nil {
			registerPWAIcons(engineInstance.Deps.Render, engineInstance.Cfg.OutputDir)
		} else {
			engineInstance.Deps.Logger.Error("Failed to write PWA icons", "error", writeError)
			return writeError
		}
		engineInstance.writeGeneratedPWAIcons(cacheDir, currentHash, icons)
		return nil
	}

	return engineInstance.copyCachedPWAIcons(cache192, cache512)
}

func (engineInstance *Engine) generatePWA(ctx context.Context, shouldForce bool) error {
	if engineInstance.Cfg.IsDev {
		return nil
	}

	errorGroup, _ := errgroup.WithContext(ctx)

	errorGroup.Go(func() error {
		return engineInstance.generateServiceWorker(shouldForce)
	})

	errorGroup.Go(func() error {
		return engineInstance.generateManifest(shouldForce)
	})

	errorGroup.Go(func() error {
		return engineInstance.generatePWAIcons(shouldForce)
	})

	return errorGroup.Wait()
}
