package renderer

import (
	"log/slog"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// SetAssets snapshots the asset map for template rendering.
func (r *Renderer) SetAssets(assets map[string]string) {
	// Create snapshot
	snapshot := make(map[string]string, len(assets))
	for k, v := range assets {
		snapshot[k] = v
	}
	r.assetsSnapshot.Store(&snapshot)
	slog.Debug("Asset map snapshot updated", "count", len(snapshot))

	// Invalidate relativization cache because assets have changed
	r.assetCache.Range(func(key, value any) bool {
		r.assetCache.Delete(key)
		return true
	})
}

// PreparePageData performs common optimizations like asset map relativization
func (r *Renderer) PreparePageData(data *models.PageData) {
	if data.Assets == nil {
		data.Assets = r.GetAssets()
	}

	// Optimization: Use cached relativized asset maps to save massive allocation churn
	if len(data.Assets) > 0 {
		cacheKey := data.BaseURL + "|" + data.RelativePrefix
		if cached, ok := r.assetCache.Load(cacheKey); ok {
			if assets, ok := cached.(map[string]string); ok {
				data.Assets = assets
			}
		} else {
			relativizedAssets := make(map[string]string, len(data.Assets))
			for k, v := range data.Assets {
				if isExternalURL(v) || isDataURI(v) {
					relativizedAssets[k] = v
					continue
				}

				relativizedAsset := relativizeAsset(v, data.BaseURL, data.RelativePrefix)
				relativizedAssets[k] = filepath.ToSlash(relativizedAsset)
			}
			r.assetCache.Store(cacheKey, relativizedAssets)
			data.Assets = relativizedAssets
		}
	}
}

// isExternalURL returns true if the URL is an absolute/external URL.
func isExternalURL(url string) bool {
	if len(url) < 5 {
		return false
	}
	return url[:5] == "http:" || url[:5] == "https"
}

// isDataURI returns true if the URL is a data URI.
func isDataURI(url string) bool {
	return len(url) >= 5 && url[:5] == "data:"
}

// relativizeAsset applies BaseURL and RelativePrefix to an asset path.
func relativizeAsset(assetPath, baseURL, relativePrefix string) string {
	if assetPath == "" {
		return ""
	}

	// Don't modify external URLs or data URIs
	if isExternalURL(assetPath) || isDataURI(assetPath) {
		return assetPath
	}

	// If baseURL is provided, prepend it
	if baseURL != "" {
		return baseURL + assetPath
	}

	// If relativePrefix is provided, prepend it (for moving up directories)
	// But first, strip leading slash from assetPath to avoid double slashes
	if relativePrefix != "" {
		if len(assetPath) > 0 && assetPath[0] == '/' {
			return relativePrefix + assetPath[1:]
		}
		return relativePrefix + assetPath
	}

	// Otherwise, remove leading slash if present (for root-relative paths)
	if len(assetPath) > 0 && assetPath[0] == '/' {
		return assetPath[1:]
	}

	return assetPath
}

// GetAssets returns a copy of the asset map to prevent accidental mutation
// of the shared global cache state. Maps are reference types in Go, so
// returning the underlying map directly would allow callers to mutate it.
func (r *Renderer) GetAssets() map[string]string {
	s := r.assetsSnapshot.Load()
	if s == nil {
		return make(map[string]string)
	}
	// Return a copy to prevent mutation of shared state
	result := make(map[string]string, len(*s))
	for k, v := range *s {
		result[k] = v
	}
	return result
}
