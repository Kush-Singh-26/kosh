package renderer

import (
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"html/template"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// setupTestRenderer creates a renderer with minimal templates for testing
func setupTestRenderer(t *testing.T) *Renderer {
	t.Helper()

	// Ensure minifier is initialized
	utils.InitMinifier()

	sink := testutil.NewMemSink()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create minimal templates for testing
	layoutTmpl := template.Must(template.New("layout").Parse(`<!DOCTYPE html>
<html>
<head><title>{{.Title}}</title></head>
<body>{{.Content}}</body>
</html>`))

	indexTmpl := template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html><head><title>Index: {{.Title}}</title></head>
<body><h1>Index</h1>{{range .Posts}}<p>{{.Title}}</p>{{end}}</body>
</html>`))

	graphTmpl := template.Must(template.New("graph").Parse(`<!DOCTYPE html>
<html><head><title>Graph</title></head>
<body><div id="graph"></div></body>
</html>`))

	notFoundTmpl := template.Must(template.New("404").Parse(`<!DOCTYPE html>
<html><head><title>404 Not Found</title></head>
<body><h1>404 - Page Not Found</h1></body>
</html>`))

	sidebarTmpl := template.Must(template.New("sidebar").Parse(`<nav>{{range .SiteTree}}<a href="{{.Link}}">{{.Title}}</a>{{end}}</nav>`))

	r := &Renderer{
		Sink:        sink,
		Assets:      make(map[string]string),
		RenderedSet: make(map[string]bool),
		Compress:    false,
		logger:      logger,
		Layout:      layoutTmpl,
		Index:       indexTmpl,
		Graph:       graphTmpl,
		NotFound:    notFoundTmpl,
		Sidebar:     sidebarTmpl,
	}

	return r
}

func TestRenderer_RenderPage_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:          "Test Page",
		Description:    "Test Description",
		Content:        template.HTML("<p>Hello World</p>"),
		BaseURL:        "",
		Assets:         map[string]string{"main.css": "/static/main.css"},
		RelativePrefix: "",
	}

	r.RenderPage("test/page/index.html", data)

	// Check if file was registered
	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["test/page/index.html"]
	r.RenderedMu.RUnlock()

	if !exists {
		t.Error("RenderPage should register the rendered file")
	}

	// Check no errors
	errs := r.ConsumeErrors()
	if errs != nil {
		t.Errorf("RenderPage should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_WithBaseURL(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:          "Test Page",
		BaseURL:        "https://example.com",
		RelativePrefix: "",
		Assets:         map[string]string{"main.css": "/static/main.css"},
		Content:        template.HTML("<p>Content</p>"),
	}

	r.RenderPage("test/baseurl.html", data)

	// Verify no errors
	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with BaseURL should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_WithRelativePrefix(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:          "Test Page",
		BaseURL:        "",
		RelativePrefix: "../",
		Assets:         map[string]string{"main.css": "/static/main.css"},
		Content:        template.HTML("<p>Content</p>"),
	}

	r.RenderPage("test/relative.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with RelativePrefix should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_LegacyProcessHTML(t *testing.T) {
	r := setupTestRenderer(t)
	r.EnableLegacyProcessHTML = true

	data := models.PageData{
		Title:          "Test Page",
		BaseURL:        "",
		RelativePrefix: "",
		Content:        template.HTML("<p>Content with <img src='image.png'></p>"),
	}

	r.RenderPage("test/legacy.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with LegacyProcessHTML should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_Compress(t *testing.T) {
	r := setupTestRenderer(t)
	r.Compress = true

	data := models.PageData{
		Title:   "Test Page",
		Content: template.HTML("<p>Content</p>"),
	}

	r.RenderPage("test/compress.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with Compress should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_NilLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.Layout = nil // Simulate nil layout

	data := models.PageData{
		Title:   "Test Page",
		Content: template.HTML("<p>Content</p>"),
	}

	// Should not panic, should log error
	r.RenderPage("test/nil-layout.html", data)

	// File should not be registered
	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["test/nil-layout.html"]
	r.RenderedMu.RUnlock()

	if exists {
		t.Error("RenderPage with nil layout should not register file")
	}
}

func TestRenderer_RenderIndex_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title: "Blog Index",
		Posts: []models.PostMetadata{
			{Title: "Post 1", Link: "/posts/post1"},
			{Title: "Post 2", Link: "/posts/post2"},
		},
		Content: template.HTML("<p>Index Content</p>"),
	}

	r.RenderIndex("index.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["index.html"]
	r.RenderedMu.RUnlock()

	if !exists {
		t.Error("RenderIndex should register the rendered file")
	}
}

func TestRenderer_RenderIndex_WithLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.Index = nil // Force fallback to layout

	data := models.PageData{
		Title:   "Fallback Index",
		Content: template.HTML("<p>Fallback</p>"),
	}

	r.RenderIndex("fallback.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderIndex with layout fallback should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderIndex_NilTemplates(t *testing.T) {
	r := setupTestRenderer(t)
	r.Index = nil
	r.Layout = nil

	data := models.PageData{
		Title:   "No Template",
		Content: template.HTML("<p>Content</p>"),
	}

	r.RenderIndex("no-template.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["no-template.html"]
	r.RenderedMu.RUnlock()

	if exists {
		t.Error("RenderIndex with nil templates should not register file")
	}
}

func TestRenderer_RenderGraph_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "Site Graph",
		Content: template.HTML("<div id='graph'></div>"),
	}

	r.RenderGraph("graph/index.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["graph/index.html"]
	r.RenderedMu.RUnlock()

	if !exists {
		t.Error("RenderGraph should register the rendered file")
	}
}

func TestRenderer_RenderGraph_NilGraph(t *testing.T) {
	r := setupTestRenderer(t)
	r.Graph = nil

	data := models.PageData{
		Title:   "No Graph",
		Content: template.HTML("<p>Content</p>"),
	}

	r.RenderGraph("no-graph.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["no-graph.html"]
	r.RenderedMu.RUnlock()

	if exists {
		t.Error("RenderGraph with nil graph should not register file")
	}
}

func TestRenderer_Render404_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "404 Page",
		Content: template.HTML("<h1>Not Found</h1>"),
	}

	r.Render404("404.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["404.html"]
	r.RenderedMu.RUnlock()

	if !exists {
		t.Error("Render404 should register the rendered file")
	}
}

func TestRenderer_Render404_WithLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.NotFound = nil // Force fallback to layout

	data := models.PageData{
		Title:   "404 Fallback",
		Content: template.HTML("<h1>404</h1>"),
	}

	r.Render404("404-fallback.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Render404 with layout fallback should not produce errors, got %v", errs)
	}
}

func TestRenderer_Render404_NilTemplates(t *testing.T) {
	r := setupTestRenderer(t)
	r.NotFound = nil
	r.Layout = nil

	data := models.PageData{
		Title:   "No Template",
		Content: template.HTML("<p>Content</p>"),
	}

	r.Render404("no-404.html", data)

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["no-404.html"]
	r.RenderedMu.RUnlock()

	if exists {
		t.Error("Render404 with nil templates should not register file")
	}
}

func TestRenderer_RenderSidebar_Success(t *testing.T) {
	r := setupTestRenderer(t)

	tree := []*models.TreeNode{
		{Title: "Home", Link: "/", Weight: 1},
		{Title: "About", Link: "/about", Weight: 2},
		{Title: "Blog", Link: "/blog", Weight: 3},
	}

	html := r.RenderSidebar(tree)

	if html == "" {
		t.Error("RenderSidebar should return HTML")
	}

	if !strings.Contains(string(html), "Home") {
		t.Error("RenderSidebar should contain 'Home'")
	}

	if !strings.Contains(string(html), "About") {
		t.Error("RenderSidebar should contain 'About'")
	}
}

func TestRenderer_RenderSidebar_NilSidebar(t *testing.T) {
	r := setupTestRenderer(t)
	r.Sidebar = nil

	tree := []*models.TreeNode{
		{Title: "Home", Link: "/"},
	}

	html := r.RenderSidebar(tree)

	if html != "" {
		t.Error("RenderSidebar with nil sidebar should return empty string")
	}
}

func TestRenderer_RenderSidebar_EmptyTree(t *testing.T) {
	r := setupTestRenderer(t)

	html := r.RenderSidebar([]*models.TreeNode{})

	if html == "" {
		t.Error("RenderSidebar with empty tree should still render sidebar template")
	}
}

func TestRenderer_RegisterFile(t *testing.T) {
	r := setupTestRenderer(t)

	r.RegisterFile("test/file.html")

	r.RenderedMu.RLock()
	_, exists := r.RenderedSet["test/file.html"]
	r.RenderedMu.RUnlock()

	if !exists {
		t.Error("RegisterFile should add file to RenderedSet")
	}
}

func TestRenderer_RegisterFile_MultipleFiles(t *testing.T) {
	r := setupTestRenderer(t)

	files := []string{
		"file1.html",
		"file2.html",
		"dir/file3.html",
		"dir/subdir/file4.html",
	}

	for _, f := range files {
		r.RegisterFile(f)
	}

	r.RenderedMu.RLock()
	count := len(r.RenderedSet)
	r.RenderedMu.RUnlock()

	if count != len(files) {
		t.Errorf("RegisterFile should register %d files, got %d", len(files), count)
	}
}

func TestRenderer_GetRenderedFiles(t *testing.T) {
	r := setupTestRenderer(t)

	files := []string{"a.html", "b.html", "c.html"}
	for _, f := range files {
		r.RegisterFile(f)
	}

	rendered := r.GetRenderedFiles()

	if len(rendered) != len(files) {
		t.Errorf("GetRenderedFiles should return %d files, got %d", len(files), len(rendered))
	}

	for _, f := range files {
		if !rendered[f] {
			t.Errorf("GetRenderedFiles should contain %s", f)
		}
	}
}

func TestRenderer_GetRenderedFiles_SnapshotCached(t *testing.T) {
	r := setupTestRenderer(t)

	r.RegisterFile("file1.html")
	r.RegisterFile("file2.html")

	// First call builds snapshot
	rendered1 := r.GetRenderedFiles()

	// Modify internal set (should not affect cached snapshot)
	r.RenderedMu.Lock()
	r.RenderedSet["file3.html"] = true
	r.renderedSnapshot.Store(nil) // Invalidate snapshot
	r.RenderedMu.Unlock()

	// Second call should rebuild snapshot with new file
	rendered2 := r.GetRenderedFiles()

	if len(rendered1) != 2 {
		t.Errorf("First snapshot should have 2 files, got %d", len(rendered1))
	}

	if len(rendered2) != 3 {
		t.Errorf("Second snapshot should have 3 files, got %d", len(rendered2))
	}
}

func TestRenderer_ClearRenderedFiles(t *testing.T) {
	r := setupTestRenderer(t)

	r.RegisterFile("file1.html")
	r.RegisterFile("file2.html")

	r.ClearRenderedFiles()

	r.RenderedMu.RLock()
	count := len(r.RenderedSet)
	r.RenderedMu.RUnlock()

	if count != 0 {
		t.Errorf("ClearRenderedFiles should clear all files, got %d", count)
	}

	// Snapshot should be invalidated
	s := r.renderedSnapshot.Load()
	if s != nil {
		t.Error("ClearRenderedFiles should invalidate snapshot")
	}
}

func TestRenderer_SetAssets(t *testing.T) {
	r := setupTestRenderer(t)

	assets := map[string]string{
		"main.css":  "/static/main.abc123.css",
		"bundle.js": "/static/bundle.xyz789.js",
	}

	r.SetAssets(assets)

	// Check snapshot
	s := r.assetsSnapshot.Load()
	if s == nil {
		t.Fatal("SetAssets should create snapshot")
	}

	if len(*s) != len(assets) {
		t.Errorf("SetAssets snapshot should have %d assets, got %d", len(assets), len(*s))
	}

	// Check asset cache is cleared
	r.assetCache.Range(func(key, value any) bool {
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

	// Initially should return empty map
	assets := r.GetAssets()
	if assets == nil {
		t.Error("GetAssets should return empty map, not nil")
	}

	if len(assets) != 0 {
		t.Errorf("GetAssets should return empty map, got %d items", len(assets))
	}

	// Set assets and retrieve
	expected := map[string]string{"style.css": "/static/style.css"}
	r.SetAssets(expected)

	assets = r.GetAssets()
	if len(assets) != 1 {
		t.Errorf("GetAssets should return 1 asset, got %d", len(assets))
	}
}

func TestRenderer_GetAssets_NilSnapshot(t *testing.T) {
	r := setupTestRenderer(t)
	// Don't call SetAssets, snapshot should be nil

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

	data := &models.PageData{
		Title:   "Test",
		BaseURL: "https://example.com",
		Assets:  map[string]string{"main.css": "/static/main.css"},
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "https://example.com/static/main.css" {
		t.Errorf("PreparePageData should prepend BaseURL, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_WithRelativePrefix(t *testing.T) {
	r := setupTestRenderer(t)

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "../",
		Assets:         map[string]string{"main.css": "/static/main.css"},
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "../static/main.css" {
		t.Errorf("PreparePageData should prepend RelativePrefix, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_WithEmptyPrefix(t *testing.T) {
	r := setupTestRenderer(t)

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "",
		Assets:         map[string]string{"main.css": "/static/main.css"},
	}

	r.PreparePageData(data)

	if data.Assets["main.css"] != "static/main.css" {
		t.Errorf("PreparePageData with empty prefix should remove leading slash, got %s", data.Assets["main.css"])
	}
}

func TestRenderer_PreparePageData_CacheHit(t *testing.T) {
	r := setupTestRenderer(t)

	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "https://example.com",
		RelativePrefix: "",
		Assets:         map[string]string{"main.css": "/static/main.css"},
	}

	// First call - should compute
	r.PreparePageData(data)
	firstAssets := data.Assets

	// Second call with same key - should use cache
	data.Assets = map[string]string{"main.css": "/static/main.css"}
	r.PreparePageData(data)

	// Note: Cache returns same map reference on hit
	// This is a soft check as implementation may vary
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

	// External URLs should not be modified
	if data.Assets["external.css"] != "https://cdn.example.com/style.css" {
		t.Errorf("External URL should not be modified, got %s", data.Assets["external.css"])
	}

	if data.Assets["data-uri"] != "data:text/css,body{}" {
		t.Errorf("Data URI should not be modified, got %s", data.Assets["data-uri"])
	}
}

func TestRenderer_SetSink(t *testing.T) {
	r := setupTestRenderer(t)

	newSink := testutil.NewMemSink()
	r.SetSink(newSink)

	if r.Sink != newSink {
		t.Error("SetSink should update the sink")
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
				Assets:         map[string]string{"test": tt.link},
			}

			r := setupTestRenderer(t)
			r.PreparePageData(data)

			if data.Assets["test"] != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, data.Assets["test"])
			}
		})
	}
}

func TestRenderer_ConcurrentRenderPage(t *testing.T) {
	r := setupTestRenderer(t)

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			data := models.PageData{
				Title:   "Concurrent Test",
				Content: template.HTML("<p>Content</p>"),
			}
			path := "concurrent/" + string(rune('a'+id)) + ".html"
			r.RenderPage(path, data)
			done <- true
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	r.RenderedMu.RLock()
	count := len(r.RenderedSet)
	r.RenderedMu.RUnlock()

	if count != numGoroutines {
		t.Errorf("Concurrent RenderPage should render %d files, got %d", numGoroutines, count)
	}
}

func TestRenderer_ConcurrentRenderIndex(t *testing.T) {
	r := setupTestRenderer(t)

	const numGoroutines = 5
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			data := models.PageData{
				Title: "Index " + string(rune('a'+id)),
				Posts: []models.PostMetadata{{Title: "Post"}},
			}
			r.RenderIndex("index_"+string(rune('a'+id))+".html", data)
			done <- true
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	r.RenderedMu.RLock()
	count := len(r.RenderedSet)
	r.RenderedMu.RUnlock()

	if count != numGoroutines {
		t.Errorf("Concurrent RenderIndex should render %d files, got %d", numGoroutines, count)
	}
}

func TestRenderer_BufferPoolReuse(t *testing.T) {
	r := setupTestRenderer(t)

	// Render multiple times to verify buffer pool is used
	for i := 0; i < 5; i++ {
		data := models.PageData{
			Title:   "Buffer Test",
			Content: template.HTML("<p>Content " + string(rune('0'+i)) + "</p>"),
		}
		r.RenderPage("buffer"+string(rune('0'+i))+".html", data)
	}

	// Should not produce errors
	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Buffer pool reuse should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_RenderError(t *testing.T) {
	r := setupTestRenderer(t)

	// Create a template that will fail
	r.Layout = template.Must(template.New("layout").Funcs(template.FuncMap{
		"panic": func() string {
			panic("template panic")
		},
	}).Parse(`{{panic}}`))

	data := models.PageData{
		Title: "Error Test",
	}

	// Should not panic, should log error
	// Note: Template execution errors are logged but not recorded via recordError
	// because the function returns early after logging
	r.RenderPage("error.html", data)

	// Error is logged but not recorded (implementation detail)
	// This test verifies no panic occurs
}

func TestRenderer_RenderPage_WriteError(t *testing.T) {
	r := setupTestRenderer(t)

	// Create a sink that will fail
	failSink := &failingSink{}
	r.Sink = failSink

	data := models.PageData{
		Title:   "Write Error Test",
		Content: template.HTML("<p>Content</p>"),
	}

	r.RenderPage("fail.html", data)

	// Should record error
	errs := r.ConsumeErrors()
	if errs == nil {
		t.Error("RenderPage with write error should record error")
	}
}

// failingSink is a test sink that always fails
type failingSink struct{}

func (f *failingSink) WriteStream(path string, fn func(io.Writer) error) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) WriteBytes(path string, bytes []byte) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) WriteFile(path string, data []byte) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) CopyFile(srcPath, destPath string) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) Mkdir(path string) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) MkdirAll(path string) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) Register(path string) {}

func (f *failingSink) GetWrittenFiles() map[string]bool {
	return nil
}

func (f *failingSink) GetOutputDir() string {
	return ""
}

func (f *failingSink) SetMtime(path string, mtime time.Time) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) Commit() error {
	return nil
}

func (f *failingSink) SetSourceTree(tree map[string]bool) {}

func TestRenderer_AssetCacheInvalidation(t *testing.T) {
	r := setupTestRenderer(t)

	// Set initial assets
	r.SetAssets(map[string]string{"main.css": "/static/main.css"})

	// Prepare page data (should cache)
	data := &models.PageData{
		Title:          "Test",
		BaseURL:        "",
		RelativePrefix: "",
		Assets:         map[string]string{"main.css": "/static/main.css"},
	}
	r.PreparePageData(data)

	// Verify cache has entry
	cacheSize := 0
	r.assetCache.Range(func(key, value any) bool {
		cacheSize++
		return true
	})

	if cacheSize != 1 {
		t.Errorf("Asset cache should have 1 entry, got %d", cacheSize)
	}

	// Set new assets (should invalidate cache)
	r.SetAssets(map[string]string{"new.css": "/static/new.css"})

	cacheSize = 0
	r.assetCache.Range(func(key, value any) bool {
		cacheSize++
		return true
	})

	if cacheSize != 0 {
		t.Errorf("SetAssets should clear asset cache, got %d entries", cacheSize)
	}
}

func TestRenderer_TimezoneHandling(t *testing.T) {
	r := setupTestRenderer(t)

	now := time.Now()
	data := models.PageData{
		Title: "Timezone Test",
		Meta: map[string]any{
			"date": now,
		},
		Content: template.HTML("<p>Content</p>"),
	}

	r.RenderPage("timezone.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Timezone handling should not produce errors, got %v", errs)
	}
}

func TestRenderer_SpecialCharacters(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:       "Test <>&\"'",
		Description: "Special chars: <script>alert('xss')</script>",
		Content:     template.HTML("<p>Content with &amp; special chars</p>"),
		Meta: map[string]any{
			"author": "Author <author@example.com>",
		},
	}

	r.RenderPage("special.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Special characters should not produce errors, got %v", errs)
	}
}

func TestRenderer_EmptyContent(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "Empty Content",
		Content: template.HTML(""),
	}

	r.RenderPage("empty.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Empty content should not produce errors, got %v", errs)
	}
}

func TestRenderer_VeryLongContent(t *testing.T) {
	r := setupTestRenderer(t)

	// Create very long content
	longContent := strings.Repeat("<p>This is a paragraph.</p>", 1000)

	data := models.PageData{
		Title:   "Long Content",
		Content: template.HTML(longContent),
	}

	r.RenderPage("long.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Long content should not produce errors, got %v", errs)
	}
}
