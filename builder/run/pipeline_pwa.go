package run

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"my-ssg/builder/generators"
	"sync"

	"github.com/spf13/afero"
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
		if b.cfg.IsDev {
			return
		}
		_ = generators.GenerateManifest(b.DestFs, "public", b.cfg.BaseURL, b.cfg.Title, b.cfg.Description, shouldForce)
	}()
	go func() {
		defer wg.Done()
		if b.cfg.IsDev {
			return
		}
		faviconPath := ""
		if b.cfg.Logo != "" {
			faviconPath = b.cfg.Logo
		} else {
			faviconPath = filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
		}

		// Ensure info is available
		if exists, _ := afero.Exists(b.SourceFs, faviconPath); !exists {
			return
		}
		srcInfo, _ := b.SourceFs.Stat(faviconPath)

		// Calculate hash based on favicon mtime and size
		hashContent := fmt.Sprintf("%s-%d-%d", faviconPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
		h := md5.New()
		h.Write([]byte(hashContent))
		currentHash := hex.EncodeToString(h.Sum(nil))

		// Check cache
		cacheDir := ".kosh-cache/pwa-icons"
		cacheHashFile := filepath.Join(cacheDir, currentHash+".hash")

		// Check if cached icons exist and are valid
		needsGeneration := false
		_, hashErr := os.Stat(cacheHashFile)
		_, icon192Err := os.Stat("public/static/images/icon-192.png")
		_, icon512Err := os.Stat("public/static/images/icon-512.png")

		// Generate if: force=true, OR hash file missing, OR icons missing
		if shouldForce || os.IsNotExist(hashErr) || os.IsNotExist(icon192Err) || os.IsNotExist(icon512Err) {
			needsGeneration = true
		}

		if needsGeneration {
			// Generate icons only if source is newer or cache is missing
			err := generators.GeneratePWAIcons(b.SourceFs, b.DestFs, faviconPath, "public/static/images")
			if err == nil {
				// Save hash to cache
				_ = os.WriteFile(cacheHashFile, []byte(currentHash), 0644)

				// Copy generated icons to cache for future reuse
				if data, err := afero.ReadFile(b.DestFs, "public/static/images/icon-192.png"); err == nil {
					_ = os.WriteFile(filepath.Join(cacheDir, currentHash+"-192.png"), data, 0644)
				}
				if data, err := afero.ReadFile(b.DestFs, "public/static/images/icon-512.png"); err == nil {
					_ = os.WriteFile(filepath.Join(cacheDir, currentHash+"-512.png"), data, 0644)
				}
			}
		} else {
			// Copy from cache to destination
			cache192 := filepath.Join(cacheDir, currentHash+"-192.png")
			cache512 := filepath.Join(cacheDir, currentHash+"-512.png")

			// Copy cached icons to VFS
			if data, err := os.ReadFile(cache192); err == nil {
				_ = afero.WriteFile(b.DestFs, "public/static/images/icon-192.png", data, 0644)
			}
			if data, err := os.ReadFile(cache512); err == nil {
				_ = afero.WriteFile(b.DestFs, "public/static/images/icon-512.png", data, 0644)
			}
		}
	}()
	wg.Wait()
}
