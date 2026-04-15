//go:build !wasm

package renderer

import (
	"html/template"
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
func (m MockConfig) GetFeatures() models.FeaturesConfig     { return models.FeaturesConfig{} }
func (m MockConfig) GetSiteTitle() string                   { return "Kosh Site" }
func (m MockConfig) GetLogo() string                        { return "" }
func (m MockConfig) GetBaseURL() string                     { return "" }
func (m MockConfig) GetContentPrefix() string               { return "" }
func (m MockConfig) GetTemplateDir() string                 { return "" }
func (m MockConfig) GetStaticDir() string                   { return "" }
func (m MockConfig) GetLayoutsDir() string                  { return "" }
func (m MockConfig) GetContentDir() string                  { return "" }
func (m MockConfig) IsDevMode() bool                        { return false }
func (m MockConfig) GetSiteData() map[string]any            { return nil }
func (m MockConfig) GetNavbar() models.NavbarIdentityConfig { return models.NavbarIdentityConfig{} }

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

// PrepareAssets performs common optimizations like asset map relativization,
// site data setup, and context detection. It is non-recursive and safe to call
// from fragment rendering.
func (r *Renderer) PrepareAssets(data *models.PageData) {
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

	// Robust Context Detection: Ensure root index always uses Home identity
	if data.Context == "" || (data.IsIndex && data.RelativePrefix == "") {
		// If at root and not explicitly a content sub-page, use Home
		if data.RelativePrefix == "" || data.RelativePrefix == "./" {
			data.Context = models.ContextHome
		}
	}

	// Universal Fragment Initialization
	if data.Fragments == nil {
		data.Fragments = make(map[string]template.HTML)
	}

	// Always setup data-structures before fragment rendering
	if data.Navbar.Title == "" {
		setupNavbar(data, r.logger)
	}

}

// PreparePageData performs full page preparation including global fragment pre-rendering.
func (r *Renderer) PreparePageData(data *models.PageData) {
	r.PrepareAssets(data)

	// Classic Path Optimization: Pre-render fragments
	contextKey := string(data.Context)

	// 1. Navbar Identity
	if data.Navbar.IdentityHTML == "" {
		fragment, err := r.RenderFragment(contextKey, "navbar-identity", *data)
		if err == nil {
			data.Navbar.IdentityHTML = fragment
			data.Fragments["navbar-identity"] = fragment
		}
	}

	// 2. Footer
	if _, ok := data.Fragments["footer"]; !ok {
		fragment, err := r.RenderFragment(contextKey, "footer", *data)
		if err == nil {
			data.Fragments["footer"] = fragment
		}
	}
}

func setupNavbar(data *models.PageData, logger *slog.Logger) {
	cfg := data.Config
	navCfg := cfg.GetNavbar()
	contentPrefix := strings.Trim(cfg.GetContentPrefix(), "/")

	// Determine context: enforce home context for root index and graph pages
	ctx := data.Context
	if ctx == "" {
		if data.IsIndex && (data.RelativePrefix == "" || data.RelativePrefix == "./") {
			ctx = models.ContextHome
		} else if data.IsGraphPage {
			ctx = models.ContextHome
		} else {
			ctx = models.ContextPosts
		}
	}

	// Apply branding from Config based on context
	if ctx == models.ContextHome {
		data.Navbar.Title = navCfg.Home.Title
		data.Navbar.BtnLabel = navCfg.Home.BtnLabel
		data.Navbar.TitleURL = "/"
		if contentPrefix != "" {
			data.Navbar.BtnURL = "/" + contentPrefix + "/"
		} else {
			data.Navbar.BtnURL = "/"
		}
	} else {
		data.Navbar.Title = navCfg.Posts.Title
		data.Navbar.BtnLabel = navCfg.Posts.BtnLabel
		if contentPrefix != "" {
			data.Navbar.TitleURL = "/" + contentPrefix + "/"
		} else {
			data.Navbar.TitleURL = "/"
		}
		data.Navbar.BtnURL = "/"
	}

	// Fallback to Site Title if navbar title is empty in config
	if data.Navbar.Title == "" {
		data.Navbar.Title = cfg.GetSiteTitle()
	}

	// Apply RelativePrefix to button URL only if it's a relative path (doesn't start with /)
	if data.RelativePrefix != "" && !strings.HasPrefix(data.Navbar.BtnURL, "/") && !strings.HasPrefix(data.Navbar.BtnURL, "http") {
		prefix := data.RelativePrefix
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		url := strings.TrimPrefix(data.Navbar.BtnURL, "/")
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
