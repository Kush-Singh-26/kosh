package renderer

import (
	"html/template"
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/renderer/base"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

// setupTestRenderer creates a renderer with minimal templates for testing.
func setupTestRenderer(t *testing.T) *Renderer {
	t.Helper()

	minify.InitHTMLMinifier()

	sink := testutil.NewMemSink()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	baseTmpl := template.Must(template.New("base.html").Funcs(templateFuncMap()).Parse(base.BaseTemplate))

	loadStub := func(content string) *template.Template {
		cloned := template.Must(baseTmpl.Clone())
		template.Must(cloned.New("stub").Parse(content))
		return cloned
	}

	layoutTmpl := loadStub(`{{define "content"}}{{.Content}}{{end}}`)
	indexTmpl := loadStub(`{{define "content"}}<h1>Index</h1>{{range .Posts}}<p>{{.Title}}</p>{{end}}{{end}}`)
	graphTmpl := template.Must(baseTmpl.Clone()) // Graph is base-only
	notFoundTmpl := loadStub(`{{define "content"}}<h1>404 - Page Not Found</h1>{{end}}`)

	r := &Renderer{
		Sink:     sink,
		Compress: false,
		logger:   logger,
		Layout:   layoutTmpl,
		Index:    indexTmpl,
		Graph:    graphTmpl,
		NotFound: notFoundTmpl,
	}

	return r
}
