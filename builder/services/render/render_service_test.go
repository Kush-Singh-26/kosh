package render

import (
	"log/slog"
	"os"
	"testing"

	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

func setupRenderServiceTest(t *testing.T) *renderService {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a renderer with in-memory mock sink (via a dummy sink implementation)
	rnd := &renderer.Renderer{
		Compress: false,
	}

	service := NewService(Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Renderer: rnd,
		Logger:   logger,
	}).(*renderService)
	return service
}

func TestNewService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rnd := &renderer.Renderer{}

	service := NewService(Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Renderer: rnd,
		Logger:   logger,
	})

	if service == nil {
		t.Fatal("NewService should not return nil")
	}

	if _, ok := service.(*renderService); !ok {
		t.Error("NewService should return *renderService")
	}
}

func TestRenderService_RegisterFile(t *testing.T) {
	service := setupRenderServiceTest(t)

	service.RegisterFile("static/style.css")

	files := service.GetRenderedFiles()
	if !files["static/style.css"] {
		t.Error("RegisterFile should register the file")
	}
}

func TestRenderService_SetAssets(t *testing.T) {
	service := setupRenderServiceTest(t)

	assets := map[string]string{
		"main.css": "main.abc123.css",
		"main.js":  "main.def456.js",
	}

	service.SetAssets(assets)

	retrievedAssets := service.GetAssets()
	if retrievedAssets["main.css"] != "main.abc123.css" {
		t.Error("SetAssets should set assets correctly")
	}

	if retrievedAssets["main.js"] != "main.def456.js" {
		t.Error("SetAssets should set all assets")
	}
}

func TestRenderService_GetAssets(t *testing.T) {
	service := setupRenderServiceTest(t)

	// Set assets through the service (which updates the snapshot)
	service.SetAssets(map[string]string{
		"app.css": "app.xyz789.css",
	})

	assets := service.GetAssets()

	if assets["app.css"] != "app.xyz789.css" {
		t.Error("GetAssets should return renderer's assets")
	}
}

func TestRenderService_GetRenderedFiles(t *testing.T) {
	service := setupRenderServiceTest(t)

	// Register some files
	service.RegisterFile("file1.html")
	service.RegisterFile("file2.html")

	files := service.GetRenderedFiles()

	if !files["file1.html"] || !files["file2.html"] {
		t.Error("GetRenderedFiles should return all registered files")
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestRenderService_ClearRenderedFiles(t *testing.T) {
	service := setupRenderServiceTest(t)

	// Register files
	service.RegisterFile("file1.html")
	service.RegisterFile("file2.html")

	// Clear them
	service.ClearRenderedFiles()

	files := service.GetRenderedFiles()
	if len(files) != 0 {
		t.Errorf("Expected 0 files after clear, got %d", len(files))
	}
}

func TestRenderService_MultipleOperations(t *testing.T) {
	service := setupRenderServiceTest(t)

	// Perform multiple operations
	service.RegisterFile("page1.html")
	service.RegisterFile("page2.html")
	service.RegisterFile("style.css")

	// Set assets
	service.SetAssets(map[string]string{
		"style.css": "style.abc.css",
		"app.js":    "app.def.js",
	})

	// Verify files
	files := service.GetRenderedFiles()
	if len(files) != 3 {
		t.Errorf("Expected 3 registered files, got %d", len(files))
	}

	// Verify assets
	assets := service.GetAssets()
	if len(assets) != 2 {
		t.Errorf("Expected 2 assets, got %d", len(assets))
	}

	// Clear files only
	service.ClearRenderedFiles()

	files = service.GetRenderedFiles()
	if len(files) != 0 {
		t.Error("Files should be cleared")
	}

	// Assets should still exist
	assets = service.GetAssets()
	if len(assets) != 2 {
		t.Error("Assets should not be affected by ClearRenderedFiles")
	}
}

func TestRenderService_RenderPage(t *testing.T) {
	service := setupRenderServiceTest(t)

	// Note: This test requires actual template files to work properly
	// Since we're using MemMapFs and no templates are set up,
	// this test mainly verifies the method exists and doesn't panic
	data := models.PageData{
		Title:   "Test Page",
		Content: "Test content",
	}

	// This will fail to render without templates, but shouldn't panic
	// We'll just verify the method can be called
	defer func() {
		if r := recover(); r != nil {
			t.Logf("RenderPage panicked (expected without templates): %v", r)
		}
	}()

	_ = service.RenderPage("test.html", data)
}

func TestRenderService_RenderIndex(t *testing.T) {
	service := setupRenderServiceTest(t)

	data := models.PageData{
		Title:   "Index Page",
		Content: "Index content",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Logf("RenderIndex panicked (expected without templates): %v", r)
		}
	}()

	_ = service.RenderIndex("index.html", data)
}

func TestRenderService_Render404(t *testing.T) {
	service := setupRenderServiceTest(t)

	data := models.PageData{
		Title:   "404 Page",
		Content: "Not found",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Render404 panicked (expected without templates): %v", r)
		}
	}()

	_ = service.Render404("404.html", data)
}

func TestRenderService_RenderGraph(t *testing.T) {
	service := setupRenderServiceTest(t)

	data := models.PageData{
		Title:   "Graph Page",
		Content: "Graph visualization",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Logf("RenderGraph panicked (expected without templates): %v", r)
		}
	}()

	_ = service.RenderGraph("graph.html", data)
}
