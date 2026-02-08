package generators

import (
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
)

// GenerateSW creates the service worker only if needed (smart build)
func GenerateSW(destFs afero.Fs, destDir string, buildVersion int64, forceRebuild bool, baseURL string, assets map[string]string) error {
	swPath := filepath.Join(destDir, "sw.js")

	// 1. Smart Check: If not forcing rebuild and SW exists, skip
	if !forceRebuild {
		if exists, _ := afero.Exists(destFs, swPath); exists {
			return nil
		}
	}

	swTemplate := `
const CACHE_NAME = 'kush-blog-cache-v{{ .Version }}';
const STATIC_CACHE = 'kush-blog-static-v{{ .Version }}';

// Dev hostnames to disable caching
const DEV_HOSTS = ['localhost', '127.0.0.1', '0.0.0.0'];

// Core app shell assets
const CORE_ASSETS = [
    '{{ .BaseURL }}/',
    '{{ .BaseURL }}/index.html',
    '{{ .BaseURL }}/404.html',
    '{{ .BaseURL }}/manifest.json'{{ range .CriticalAssets }},
    '{{ $.BaseURL }}{{ . }}'{{ end }}
];
`

	tmpl, err := template.New("sw").Parse(swTemplate)
	if err != nil {
		return err
	}

	if err := destFs.MkdirAll(filepath.Dir(swPath), 0755); err != nil {
		return err
	}
	f, err := destFs.Create(swPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	data := struct {
		Version        int64
		BaseURL        string
		CriticalAssets []string
	}{
		Version: buildVersion,
		BaseURL: baseURL,
	}

	// Identify critical assets to pre-cache
	criticalKeys := []string{
		"/static/css/layout.css",
		"/static/css/theme.css",
		"/static/js/main.js",
		"/static/js/search.js",
		"/static/js/wasm_exec.js",
	}

	for _, key := range criticalKeys {
		if val, ok := assets[key]; ok {
			data.CriticalAssets = append(data.CriticalAssets, val)
		} else {
			// Fallback if assets map is empty (e.g. dev mode without hashing)
			data.CriticalAssets = append(data.CriticalAssets, key)
		}
	}

	return tmpl.Execute(f, data)
}

// GenerateManifest creates the manifest.json dynamically with a smart build check
func GenerateManifest(destFs afero.Fs, destDir string, baseURL string, siteTitle string, siteDescription string, forceRebuild bool) error {
	manifestPath := filepath.Join(destDir, "manifest.json")

	// 1. Smart Check: If not forcing rebuild and manifest exists, skip
	if !forceRebuild {
		if exists, _ := afero.Exists(destFs, manifestPath); exists {
			return nil
		}
	}

	manifestTemplate := `{
    "name": "{{ .Title }}",
    "short_name": "{{ .Title }}",
    "start_url": "./",
    "display": "standalone",
    "background_color": "#111113",
    "theme_color": "#111113",
    "description": "{{ .Description }}",
    "icons": [
        {
            "src": "static/images/icon-192.png",
            "sizes": "192x192",
            "type": "image/png",
            "purpose": "any"
        },
        {
            "src": "static/images/icon-192.png",
            "sizes": "192x192",
            "type": "image/png",
            "purpose": "maskable"
        },
        {
            "src": "static/images/icon-512.png",
            "sizes": "512x512",
            "type": "image/png",
            "purpose": "any"
        },
        {
            "src": "static/images/icon-512.png",
            "sizes": "512x512",
            "type": "image/png",
            "purpose": "maskable"
        }
    ],
    "id": "./",
    "scope": "./"
}
`

	tmpl, err := template.New("manifest").Parse(manifestTemplate)
	if err != nil {
		return err
	}

	if err := destFs.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		return err
	}
	f, err := destFs.Create(manifestPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	data := struct {
		Title       string
		Description string
		BaseURL     string
	}{
		Title:       siteTitle,
		Description: siteDescription,
		BaseURL:     baseURL,
	}

	return tmpl.Execute(f, data)
}

// GeneratePWAIcons generates 192x192 and 512x512 icons from favicon.png
func GeneratePWAIcons(srcFs afero.Fs, destFs afero.Fs, srcPath, destDir string) error {
	// Source must exist
	srcFile, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("source icon not found: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	// Open source image
	src, err := imaging.Decode(srcFile)
	if err != nil {
		return err
	}

	// Create dest dir if needed
	if err := destFs.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	sizes := []int{192, 512}

	for _, size := range sizes {
		destFile := filepath.Join(destDir, fmt.Sprintf("icon-%d.png", size))

		// Check if destination exists in VFS.
		// Note: We don't check modtime here easily against source stream without Stat'ing source path again.
		// For simplicity in VFS build (which might be fresh), just generate.
		// If we want incremental, we rely on checking if file exists in VFS (it won't if VFS is fresh).

		fmt.Printf("   🎨 Generating PWA Icon: %dx%d\n", size, size)

		// Resize
		dst := imaging.Resize(src, size, size, imaging.Lanczos)

		// Save to VFS
		f, err := destFs.Create(destFile)
		if err != nil {
			return err
		}

		// Encode as PNG
		err = imaging.Encode(f, dst, imaging.PNG)
		_ = f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
