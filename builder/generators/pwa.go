package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/disintegration/imaging"
)

// GenerateSW creates the service worker only if needed (smart build)
func GenerateSW(destDir string, buildVersion int64, forceRebuild bool, baseURL string) error {
	swPath := filepath.Join(destDir, "sw.js")

	// 1. Smart Check: If not forcing rebuild and SW exists, skip
	if !forceRebuild {
		if _, err := os.Stat(swPath); err == nil {
			return nil
		}
	}

	fmt.Println("   📱 Generating Service Worker...")

	swTemplate := `
const CACHE_NAME = 'kush-blog-cache-v{{ .Version }}';
const ASSETS = [
    '{{ .BaseURL }}/',
    '{{ .BaseURL }}/index.html',
    '{{ .BaseURL }}/404.html',
    '{{ .BaseURL }}/static/css/layout.css',
    '{{ .BaseURL }}/static/css/theme.css',
    '{{ .BaseURL }}/static/js/main.js',
    '{{ .BaseURL }}/static/images/favicon.webp',
    '{{ .BaseURL }}/static/manifest.json'
];
`

	tmpl, err := template.New("sw").Parse(swTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(swPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	data := struct {
		Version int64
		BaseURL string
	}{
		Version: buildVersion,
		BaseURL: baseURL,
	}

	return tmpl.Execute(f, data)
}

// GeneratePWAIcons generates 192x192 and 512x512 icons from favicon.png
func GeneratePWAIcons(srcPath, destDir string) error {
	// Source must exist
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source icon not found: %w", err)
	}

	// Create dest dir if needed
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	sizes := []int{192, 512}

	for _, size := range sizes {
		destFile := filepath.Join(destDir, fmt.Sprintf("icon-%d.png", size))

		// Smart Check: Skip if destination exists and is newer than source
		if destInfo, err := os.Stat(destFile); err == nil {
			if destInfo.ModTime().After(srcInfo.ModTime()) {
				continue
			}
		}

		fmt.Printf("   🎨 Generating PWA Icon: %dx%d\n", size, size)

		// Open source image
		src, err := imaging.Open(srcPath)
		if err != nil {
			return err
		}

		// Resize
		dst := imaging.Resize(src, size, size, imaging.Lanczos)

		// Save
		err = imaging.Save(dst, destFile)
		if err != nil {
			return err
		}
	}

	return nil
}
