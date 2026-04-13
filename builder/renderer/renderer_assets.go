package renderer

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

type MockConfig struct{}

func (m MockConfig) GetMenu() []models.MenuEntry       { return nil }
func (m MockConfig) GetFooterMenu() []models.MenuEntry { return nil }
func (m MockConfig) GetAuthor() models.AuthorConfig    { return models.AuthorConfig{} }
func (m MockConfig) GetSocial() models.SocialCardsConfig {
	return models.SocialCardsConfig{Gradient: []string{"#000", "#fff"}}
}
func (m MockConfig) GetFeatures() models.FeaturesConfig { return models.FeaturesConfig{} }
func (m MockConfig) GetSiteTitle() string               { return "Kosh Blog" }
func (m MockConfig) GetLogo() string                    { return "" }
func (m MockConfig) GetBaseURL() string                 { return "" }
func (m MockConfig) GetBlogPrefix() string              { return "" }
func (m MockConfig) IsDevMode() bool                    { return false }
func (m MockConfig) GetSiteData() map[string]any        { return nil }

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
	if data.Config == nil {
		data.Config = MockConfig{}
	}
	if data.TabTitle == "" && data.Title != "" {
		data.TabTitle = data.Title
	}

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

	if data.SiteData == nil {
		data.SiteData = data.Config.GetSiteData()
	}

	// Setup Navbar based on Context
	if data.Navbar.Title == "" {
		setupNavbar(data, r.logger)
	}
}

func setupNavbar(data *models.PageData, logger *slog.Logger) {
	cfg := data.Config
	blogPrefix := strings.Trim(cfg.GetBlogPrefix(), "/")

	// Determine context
	ctx := data.Context
	if ctx == "" {
		// Default based on BlogPrefix if not explicitly set
		if blogPrefix == "" || data.IsHome {
			ctx = models.ContextHome
		} else {
			ctx = models.ContextBlog
		}
	}

	// Set Navbar Title
	if ctx == models.ContextHome {
		data.Navbar.Title = cfg.GetSiteTitle()
		data.Navbar.TitleURL = "/"
	} else {
		data.Navbar.Title = "Kush Blogs"
		if blogPrefix != "" {
			data.Navbar.TitleURL = "/" + blogPrefix + "/"
		} else {
			data.Navbar.TitleURL = "/blogs/"
		}
	}

	// Set Navbar Button
	if ctx == models.ContextHome {
		data.Navbar.BtnLabel = "Blogs"
		if blogPrefix != "" {
			data.Navbar.BtnURL = "/" + blogPrefix + "/"
		} else {
			data.Navbar.BtnURL = "/blogs/"
		}
	} else {
		data.Navbar.BtnLabel = "Home"
		data.Navbar.BtnURL = "/"
	}

	// Apply RelativePrefix to button URL only if it's a relative path (doesn't start with /)
	// Absolute root paths (/) should stay as / and be treated as root in the browser
	if data.RelativePrefix != "" && !strings.HasPrefix(data.Navbar.BtnURL, "/") && !strings.HasPrefix(data.Navbar.BtnURL, "http") {
		prefix := data.RelativePrefix
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		url := data.Navbar.BtnURL
		if strings.HasPrefix(url, "/") {
			url = url[1:]
		}
		data.Navbar.BtnURL = prefix + url
	}

	logger.Debug("Navbar setup",
		"context", ctx,
		"title", data.Navbar.Title,
		"btnLabel", data.Navbar.BtnLabel,
		"btnURL", data.Navbar.BtnURL)
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

	// Handle root-relative prefix specifically
	if relativePrefix == "/" {
		return assetPath
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
