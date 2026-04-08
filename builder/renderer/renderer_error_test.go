package renderer

import (
	"html/template"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestRenderer_RenderPage_NilLayout(t *testing.T) {
	r := setupTestRenderer(t)
	r.Layout = nil

	data := models.PageData{
		Title:   "Test Page",
		Content: template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("test/nil-layout.html", data)

	exists := r.GetRenderedFiles()["test/nil-layout.html"]
	if exists {
		t.Error("RenderPage with nil layout should not register file")
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

	_ = r.RenderIndex("no-template.html", data)

	exists := r.GetRenderedFiles()["no-template.html"]
	if exists {
		t.Error("RenderIndex with nil templates should not register file")
	}
}

func TestRenderer_RenderGraph_NilGraph(t *testing.T) {
	r := setupTestRenderer(t)
	r.Graph = nil

	data := models.PageData{
		Title:   "No Graph",
		Content: template.HTML("<p>Content</p>"),
	}

	_ = r.RenderGraph("no-graph.html", data)

	exists := r.GetRenderedFiles()["no-graph.html"]
	if exists {
		t.Error("RenderGraph with nil graph should not register file")
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

	_ = r.Render404("no-404.html", data)

	exists := r.GetRenderedFiles()["no-404.html"]
	if exists {
		t.Error("Render404 with nil templates should not register file")
	}
}

func TestRenderer_RenderPage_RenderError(t *testing.T) {
	r := setupTestRenderer(t)

	r.Layout = template.Must(template.New("layout").Funcs(template.FuncMap{
		"panic": func() string {
			panic("template panic")
		},
	}).Parse(`{{panic}}`))

	data := models.PageData{
		Title: "Error Test",
	}

	_ = r.RenderPage("error.html", data)
}

func TestRenderer_RenderPage_WriteError(t *testing.T) {
	r := setupTestRenderer(t)

	failSink := &failingSink{}
	r.Sink = failSink

	data := models.PageData{
		Title:   "Write Error Test",
		Content: template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("fail.html", data)

	errs := r.ConsumeErrors()
	if errs == nil {
		t.Error("RenderPage with write error should record error")
	}
}

// failingSink is a test sink that always fails.
type failingSink struct {
	testutil.MemSink
}

func (f *failingSink) WriteStream(path string, fn func(io.Writer) error) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) WriteFile(path string, data []byte) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) CopyFile(srcPath, destPath string) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) MkdirAll(path string) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) SetMtime(path string, mtime time.Time) error {
	return io.ErrUnexpectedEOF
}

func (f *failingSink) Stat(path string) (os.FileInfo, error) {
	return nil, io.ErrUnexpectedEOF
}
