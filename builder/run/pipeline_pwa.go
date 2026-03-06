package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/spf13/afero"
)

func (b *Builder) generatePWA(ctx context.Context, shouldForce bool) error {
	if b.cfg.IsDev {
		return nil
	}

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		return generators.GenerateSW(b.Sink, b.cfg.OutputDir, b.cfg.BuildVersion, shouldForce, b.cfg.BaseURL, b.renderService.GetAssets())
	})

	g.Go(func() error {
		return generators.GenerateManifest(b.Sink, b.cfg.OutputDir, b.cfg.BaseURL, b.cfg.Title, b.cfg.Description, shouldForce)
	})

	g.Go(func() error {
		faviconPath := b.getFaviconPath()

		// Ensure info is available
		exists, _ := afero.Exists(b.SourceFs, faviconPath)
		if !exists {
			return nil
		}
		srcInfo, _ := b.SourceFs.Stat(faviconPath)

		// Calculate hash based on favicon mtime and size
		hashContent := fmt.Sprintf("%s-%d-%d", faviconPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
		currentHash := cache.HashString(hashContent)

		// Check cache
		cacheDir := filepath.Join(b.cfg.CacheDir, "pwa-icons")
		cacheHashFile := filepath.Join(cacheDir, currentHash+".hash")

		// Check if cached icons exist and are valid
		needsGeneration := false
		_, hashErr := os.Stat(cacheHashFile)
		_, icon192Err := os.Stat(filepath.Join(b.cfg.OutputDir, "static/images/icon-192.png"))
		_, icon512Err := os.Stat(filepath.Join(b.cfg.OutputDir, "static/images/icon-512.png"))

		// Generate if: force=true, OR hash file missing, OR icons missing
		if shouldForce || os.IsNotExist(hashErr) || os.IsNotExist(icon192Err) || os.IsNotExist(icon512Err) {
			needsGeneration = true
		}

		if needsGeneration {
			// Generate icons only if source is newer or cache is missing
			err := generators.GeneratePWAIcons(b.SourceFs, b.Sink, faviconPath, filepath.Join(b.cfg.OutputDir, "static/images"))
			if err == nil {
				// Save hash to cache
				_ = os.WriteFile(cacheHashFile, []byte(currentHash), 0644)

				// Copy generated icons to cache for future reuse
				if data, err := os.ReadFile(filepath.Join(b.Sink.GetOutputDir(), "static/images/icon-192.png")); err == nil {
					_ = os.WriteFile(filepath.Join(cacheDir, currentHash+"-192.png"), data, 0644)
				}
				if data, err := os.ReadFile(filepath.Join(b.Sink.GetOutputDir(), "static/images/icon-512.png")); err == nil {
					_ = os.WriteFile(filepath.Join(cacheDir, currentHash+"-512.png"), data, 0644)
				}
			}
		} else {
			// Copy from cache to destination
			cache192 := filepath.Join(cacheDir, currentHash+"-192.png")
			cache512 := filepath.Join(cacheDir, currentHash+"-512.png")

			// Copy cached icons to VFS
			if data, err := os.ReadFile(cache192); err == nil {
				iconPath := filepath.Join(b.cfg.OutputDir, "static/images/icon-192.png")
				_ = b.Sink.WriteFile(iconPath, data)
				b.renderService.RegisterFile(iconPath)
			}
			if data, err := os.ReadFile(cache512); err == nil {
				iconPath := filepath.Join(b.cfg.OutputDir, "static/images/icon-512.png")
				_ = b.Sink.WriteFile(iconPath, data)
				b.renderService.RegisterFile(iconPath)
			}
		}
		return nil
	})

	return g.Wait()
}
