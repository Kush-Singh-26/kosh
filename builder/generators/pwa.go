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
const STATIC_CACHE = 'kush-blog-static-v{{ .Version }}';

// Core app shell assets
const CORE_ASSETS = [
    '{{ .BaseURL }}/',
    '{{ .BaseURL }}/index.html',
    '{{ .BaseURL }}/404.html',
    '{{ .BaseURL }}/manifest.json'
];

// Install: Cache core assets
self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME)
            .then((cache) => cache.addAll(CORE_ASSETS))
            .then(() => self.skipWaiting())
    );
});

// Activate: Clean up old caches
self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((cacheNames) => {
            return Promise.all(
                cacheNames.map((cache) => {
                    if (cache !== CACHE_NAME && cache !== STATIC_CACHE) {
                        return caches.delete(cache);
                    }
                })
            );
        }).then(() => self.clients.claim())
    );
});

// Stale-while-revalidate strategy for better performance
self.addEventListener('fetch', (event) => {
    const { request } = event;
    const url = new URL(request.url);
    
    // Skip non-GET requests
    if (request.method !== 'GET') {
        event.respondWith(fetch(request));
        return;
    }
    
    // Strategy for HTML pages: Network first, fallback to cache
    if (request.mode === 'navigate' || request.headers.get('accept').includes('text/html')) {
        event.respondWith(
            fetch(request)
                .then((response) => {
                    // Update cache with fresh version
                    const clone = response.clone();
                    caches.open(CACHE_NAME).then((cache) => {
                        cache.put(request, clone);
                    });
                    return response;
                })
                .catch(() => {
                    // Fallback to cache if network fails
                    return caches.match(request);
                })
        );
        return;
    }
    
    // Strategy for static assets: Cache first, revalidate in background
    event.respondWith(
        caches.match(request).then((cachedResponse) => {
            // Return cached version immediately (fast!)
            const fetchPromise = fetch(request).then((networkResponse) => {
                // Update cache in background for next visit
                if (networkResponse.ok) {
                    const clone = networkResponse.clone();
                    caches.open(STATIC_CACHE).then((cache) => {
                        cache.put(request, clone);
                    });
                }
                return networkResponse;
            }).catch(() => {
                // Network failed, but we already returned cached version
                return cachedResponse;
            });
            
            return cachedResponse || fetchPromise;
        })
    );
});
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

// GenerateManifest creates the manifest.json dynamically with a smart build check
func GenerateManifest(destDir string, baseURL string, siteTitle string, siteDescription string, forceRebuild bool) error {
	manifestPath := filepath.Join(destDir, "manifest.json")

	// 1. Smart Check: If not forcing rebuild and manifest exists, skip
	if !forceRebuild {
		if _, err := os.Stat(manifestPath); err == nil {
			return nil
		}
	}

	fmt.Println("   📱 Generating Web Manifest...")

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

	f, err := os.Create(manifestPath)
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
