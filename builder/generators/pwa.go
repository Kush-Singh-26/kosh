package generators

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
)

// GenerateSW creates the service worker only if needed (smart build)
func GenerateSW(sink utils.ArtifactSink, destDir string, buildVersion int64, forceRebuild bool, baseURL string, assets map[string]string) error {
	swPath := filepath.Join(destDir, "sw.js")

	// 1. Smart Check: If not forcing rebuild and SW exists, skip
	if !forceRebuild && !TestingMode {
		if _, err := os.Stat(swPath); err == nil {
			sink.Register(swPath)
			return nil
		}
	}

	swTemplate := `
// Service worker disabled in dev mode or incomplete implementation
self.addEventListener('fetch', function(event) {
    // Pass-through
});
`

	tmpl, err := template.New("sw").Parse(swTemplate)
	if err != nil {
		return err
	}

	if err := sink.MkdirAll(filepath.Dir(swPath)); err != nil {
		return err
	}

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

	return sink.WriteStream(swPath, func(w io.Writer) error {
		return tmpl.Execute(w, data)
	})
}

// GenerateManifest creates the manifest.json dynamically with a smart build check
func GenerateManifest(sink utils.ArtifactSink, destDir string, baseURL string, siteTitle string, siteDescription string, forceRebuild bool) error {
	manifestPath := filepath.Join(destDir, "manifest.json")

	// 1. Smart Check: If not forcing rebuild and manifest exists, skip
	if !forceRebuild && !TestingMode {
		if _, err := os.Stat(manifestPath); err == nil {
			sink.Register(manifestPath)
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

	if err := sink.MkdirAll(filepath.Dir(manifestPath)); err != nil {
		return err
	}

	data := struct {
		Title       string
		Description string
		BaseURL     string
	}{
		Title:       siteTitle,
		Description: siteDescription,
		BaseURL:     baseURL,
	}

	return sink.WriteStream(manifestPath, func(w io.Writer) error {
		return tmpl.Execute(w, data)
	})
}

// PWAIconsData holds encoded icon PNG bytes keyed by icon size.
type PWAIconsData map[int][]byte

// GeneratePWAIconBytes generates 192x192 and 512x512 icon PNG bytes from the source icon.
func GeneratePWAIconBytes(srcFs afero.Fs, srcPath string) (PWAIconsData, error) {
	// Source must exist
	srcFile, err := srcFs.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("source icon not found: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	// Open source image
	src, err := imaging.Decode(srcFile)
	if err != nil {
		return nil, err
	}

	sizes := []int{192, 512}
	out := make(PWAIconsData, len(sizes))

	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]error, len(sizes))

	for i, size := range sizes {
		wg.Add(1)
		go func(idx, sz int) {
			defer wg.Done()

			fmt.Printf("   🎨 Generating PWA Icon: %dx%d\n", sz, sz)
			dst := imaging.Resize(src, sz, sz, imaging.Lanczos)

			var buf bytes.Buffer
			if err := imaging.Encode(&buf, dst, imaging.PNG); err != nil {
				errs[idx] = err
				return
			}

			encoded := make([]byte, buf.Len())
			copy(encoded, buf.Bytes())

			mu.Lock()
			out[sz] = encoded
			mu.Unlock()
		}(i, size)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// WritePWAIcons writes pre-encoded icon bytes to the destination directory via sink.
func WritePWAIcons(sink utils.ArtifactSink, destDir string, icons PWAIconsData) error {
	if err := sink.MkdirAll(destDir); err != nil {
		return err
	}

	for _, sz := range []int{192, 512} {
		data, ok := icons[sz]
		if !ok || len(data) == 0 {
			return fmt.Errorf("missing encoded icon data for %d", sz)
		}
		destFile := filepath.Join(destDir, fmt.Sprintf("icon-%d.png", sz))
		if err := sink.WriteFile(destFile, data); err != nil {
			return err
		}
	}
	return nil
}

// GeneratePWAIcons generates 192x192 and 512x512 icons from favicon.png
func GeneratePWAIcons(srcFs afero.Fs, sink utils.ArtifactSink, srcPath, destDir string) error {
	icons, err := GeneratePWAIconBytes(srcFs, srcPath)
	if err != nil {
		return err
	}
	return WritePWAIcons(sink, destDir, icons)
}
