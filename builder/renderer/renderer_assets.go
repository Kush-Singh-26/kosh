package renderer

import (
	"maps"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func (r *Renderer) SetAssets(assets map[string]string) {
	r.AssetsMu.Lock()
	r.Assets = assets
	// Create snapshot
	snapshot := make(map[string]string, len(assets))
	maps.Copy(snapshot, assets)
	r.assetsSnapshot.Store(&snapshot)
	// Invalidate relativization cache because assets have changed
	r.assetCache.Range(func(key, value any) bool {
		r.assetCache.Delete(key)
		return true
	})
	r.AssetsMu.Unlock()
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
			prefix := data.RelativePrefix
			baseURL := data.BaseURL
			for k, v := range data.Assets {
				link := v
				if link[0] != '/' {
					link = "/" + link
				}
				if baseURL != "" {
					relativizedAssets[k] = strings.TrimSuffix(baseURL, "/") + link
				} else if prefix == "" || prefix == "." || prefix == "./" {
					relativizedAssets[k] = link[1:]
				} else {
					relativizedAssets[k] = prefix + link[1:]
				}
			}
			r.assetCache.Store(cacheKey, relativizedAssets)
			data.Assets = relativizedAssets
		}
	}
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
