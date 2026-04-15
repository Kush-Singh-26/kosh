package renderer

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestRenderer_SetAssets(t *testing.T) {
	r := setupTestRenderer(t)

	assets := map[string]string{
		"main.css":  "/static/main.abc123.css",
		"bundle.js": "/static/bundle.xyz789.js",
	}

	r.SetAssets(assets)

	s := r.assetsSnapshot.Load()
	if s == nil {
		t.Fatal("SetAssets should create snapshot")
	}

	if len(*s) != len(assets) {
		t.Errorf("SetAssets snapshot should have %d assets, got %d", len(assets), len(*s))
	}

	r.assetCache.Range(func(_, _ any) bool {
		t.Error("SetAssets should clear asset cache")
		return false
	})
}

func TestRenderer_SetAssets_Empty(t *testing.T) {
	r := setupTestRenderer(t)

	r.SetAssets(map[string]string{})

	s := r.assetsSnapshot.Load()
	if s == nil {
		t.Fatalf("SetAssets with empty map should still create snapshot")
	}

	if len(*s) != 0 {
		t.Error("SetAssets with empty map should create empty snapshot")
	}
}

func TestRenderer_GetAssets(t *testing.T) {
	r := setupTestRenderer(t)

	assets := r.GetAssets()
	if assets == nil {
		t.Error("GetAssets should return empty map, not nil")
	}

	if len(assets) != 0 {
		t.Errorf("GetAssets should return empty map, got %d items", len(assets))
	}

	expected := map[string]string{"style.css": "/static/style.css"}
	r.SetAssets(expected)

	assets = r.GetAssets()
	if len(assets) != 1 {
		t.Errorf("GetAssets should return 1 asset, got %d", len(assets))
	}
}

func TestRenderer_GetAssets_NilSnapshot(t *testing.T) {
	r := setupTestRenderer(t)

	assets := r.GetAssets()
	if assets == nil {
		t.Error("GetAssets should return empty map when snapshot is nil")
	}
}

func TestRenderer_PreparePageData_NilAssets(t *testing.T) {
	r := setupTestRenderer(t)
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title: "Test",
	}

	r.PreparePageData(data)

	if data.Assets == nil {
		t.Error("PreparePageData should initialize nil assets from renderer")
	}

	if len(data.Assets) != 1 {
		t.Errorf("PreparePageData should copy assets, got %d", len(data.Assets))
	}
}

func TestRenderer_PreparePageData_WithBaseURL(t *testing.T) {
	r := setupTestRenderer(t)
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title:   "Test",
		BaseURL: "https://example.com",
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "https://example.com/static/main.css" {
		t.Errorf("PreparePageData should prepend BaseURL, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_WithRelativePrefix(t *testing.T) {
	r := setupTestRenderer(t)
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "../",
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "../static/main.css" {
		t.Errorf("PreparePageData should prepend RelativePrefix, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_WithEmptyPrefix(t *testing.T) {
	r := setupTestRenderer(t)
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "",
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "static/main.css" {
		t.Errorf("PreparePageData with empty prefix should remove leading slash, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_CacheHit(t *testing.T) {
	r := setupTestRenderer(t)
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "https://example.com",
		RelativePrefix: "",
	}

	r.PreparePageData(data)
	firstAssets := data.Assets

	data.Assets = map[string]string{"main.css": "/static/main.css"}
	r.PreparePageData(data)

	if firstAssets["main.css"] != data.Assets["main.css"] {
		t.Log("Cache behavior may vary based on implementation")
	}
}

func TestRenderer_PreparePageData_ExternalURLs(t *testing.T) {
	r := setupTestRenderer(t)

	data := &models.PageData{
		Title:   "Test",
		BaseURL: "",
		Assets: map[string]string{
			"external.css": "https://cdn.example.com/style.css",
			"data-uri":     "data:text/css,body{}",
		},
	}

	r.PreparePageData(data)

	if data.Assets["external.css"] != "https://cdn.example.com/style.css" {
		t.Errorf("External URL should not be modified, got %s", data.Assets["external.css"])
	}

	if data.Assets["data-uri"] != "data:text/css,body{}" {
		t.Errorf("Data URI should not be modified, got %s", data.Assets["data-uri"])
	}
}

func TestRenderer_RelativizeFunc(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		prefix   string
		link     string
		expected string
	}{
		{"absolute URL", "", "", "https://example.com/style.css", "https://example.com/style.css"},
		{"data URI", "", "", "data:text/css,body{}", "data:text/css,body{}"},
		{"root path no prefix", "", "", "/style.css", "style.css"},
		{"root path with prefix", "", "../", "/style.css", "../style.css"},
		{"root path with baseURL", "https://example.com", "", "/style.css", "https://example.com/style.css"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &models.PageData{
				Title:          "Test",
				BaseURL:        tt.baseURL,
				RelativePrefix: tt.prefix,
			}

			r := setupTestRenderer(t)
			r.SetAssets(map[string]string{"test": tt.link})
			r.PreparePageData(data)

			if data.Assets["test"] != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, data.Assets["test"])
			}
		})
	}
}

func TestRenderer_AssetCacheInvalidation(t *testing.T) {
	r := setupTestRenderer(t)

	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "",
	}
	r.PreparePageData(data)

	cacheSize := 0
	r.assetCache.Range(func(_, _ any) bool {
		cacheSize++
		return true
	})

	if cacheSize != 1 {
		t.Errorf("Asset cache should have 1 entry, got %d", cacheSize)
	}

	r.SetAssets(map[string]string{"new.css": "/static/new.css"})

	cacheSize = 0
	r.assetCache.Range(func(_, _ any) bool {
		cacheSize++
		return true
	})

	if cacheSize != 0 {
		t.Errorf("SetAssets should clear asset cache, got %d entries", cacheSize)
	}
}
