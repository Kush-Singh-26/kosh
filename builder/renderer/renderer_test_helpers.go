package renderer

import (
	"html/template"
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

// setupTestRenderer creates a renderer with minimal templates for testing.
func setupTestRenderer(t *testing.T) *Renderer {
	t.Helper()

	minify.InitHTMLMinifier()

	sink := testutil.NewMemSink()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

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
