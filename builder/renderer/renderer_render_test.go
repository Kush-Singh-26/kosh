package renderer

import (
	"html/template"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestRenderer_RenderPage_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:          "Test Page",
		Description:    "Test Description",
		Content:        template.HTML("<p>Hello World</p>"),
		BaseURL:        "",
		RelativePrefix: "",
	}

	_ = r.RenderPage("test/page/index.html", data)

	exists := r.GetRenderedFiles()["test/page/index.html"]
	if !exists {
		t.Error("RenderPage should register the rendered file")
	}

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_WithBaseURL(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:          "Test Page",
		BaseURL:        "https://example.com",
		RelativePrefix: "",
		Content:        template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("test/baseurl.html", data)

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
		Content:        template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("test/relative.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with RelativePrefix should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderPage_Compress(t *testing.T) {
	r := setupTestRenderer(t)
	r.Compress = true

	data := models.PageData{
		Title:   "Test Page",
		Content: template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("test/compress.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderPage with Compress should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderIndex_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title: "Item Index",
		Items: []models.ContentMetadata{
			{Title: "Item 1", Link: "/items/item1"},
			{Title: "Item 2", Link: "/items/item2"},
		},
		Content: template.HTML("<p>Index Content</p>"),
	}

	_ = r.RenderIndex("index.html", data)

	exists := r.GetRenderedFiles()["index.html"]
	if !exists {
		t.Error("RenderIndex should register the rendered file")
	}
}

func TestRenderer_RenderIndex_WithLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.Index = nil

	data := models.PageData{
		Title:   "Fallback Index",
		Content: template.HTML("<p>Fallback</p>"),
	}

	_ = r.RenderIndex("fallback.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("RenderIndex with layout fallback should not produce errors, got %v", errs)
	}
}

func TestRenderer_RenderGraph_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "Site Graph",
		Content: template.HTML("<div id='graph'></div>"),
	}

	_ = r.RenderGraph("graph/index.html", data)

	exists := r.GetRenderedFiles()["graph/index.html"]
	if !exists {
		t.Error("RenderGraph should register the rendered file")
	}
}

func TestRenderer_Render404_Success(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "404 Page",
		Content: template.HTML("<h1>Not Found</h1>"),
	}

	_ = r.Render404("404.html", data)

	exists := r.GetRenderedFiles()["404.html"]
	if !exists {
		t.Error("Render404 should register the rendered file")
	}
}

func TestRenderer_Render404_WithLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.NotFound = nil

	data := models.PageData{
		Title:   "404 Fallback",
		Content: template.HTML("<h1>404</h1>"),
	}

	_ = r.Render404("404-fallback.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Render404 with layout fallback should not produce errors, got %v", errs)
	}
}

func TestRenderer_RegisterFile(t *testing.T) {
	r := setupTestRenderer(t)

	r.RegisterFile("test/file.html")

	exists := r.GetRenderedFiles()["test/file.html"]
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

	count := len(r.GetRenderedFiles())
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

func TestRenderer_ClearRenderedFiles(t *testing.T) {
	r := setupTestRenderer(t)

	r.RegisterFile("file1.html")
	r.RegisterFile("file2.html")

	r.ClearRenderedFiles()

	count := len(r.GetRenderedFiles())
	if count != 0 {
		t.Errorf("ClearRenderedFiles should clear all files, got %d", count)
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
