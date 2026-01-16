// Handles template loading and file creation
package renderer

import (
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

type Renderer struct {
	Layout   *template.Template
	Index    *template.Template
	Graph    *template.Template
	NotFound *template.Template
	Assets   map[string]string
	Compress bool
}

func New(compress bool) *Renderer {
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"now":       time.Now,
	}

	// Load layout template
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html")
	if err != nil {
		log.Fatal(err)
	}

	// Load index template
	indexTmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("⚠️  Index template not found, will use layout.html for index. (%v)\n", err)
		indexTmpl = nil
	}

	// Load graph template
	graphTmpl, err := template.ParseFiles("templates/graph.html")
	if err != nil {
		log.Printf("⚠️  Graph template not found, skipping graph page. (%v)\n", err)
	}

	// Load 404 template
	notFoundTmpl, err := template.New("404.html").Funcs(funcMap).ParseFiles("templates/404.html")
	if err != nil {
		log.Printf("⚠️  404 template not found, will use layout.html for 404 page. (%v)\n", err)
		notFoundTmpl = nil
	}

	return &Renderer{
		Layout:   tmpl,
		Index:    indexTmpl,
		Graph:    graphTmpl,
		NotFound: notFoundTmpl,
		Compress: compress,
	}
}

func (r *Renderer) SetAssets(assets map[string]string) {
	r.Assets = assets
}

func (r *Renderer) RenderPage(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		log.Printf("❌ Failed to create file %s: %v\n", path, err)
		return
	}
	defer func() { _ = f.Close() }()

	var w io.Writer = f

	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer func() { _ = mw.Close() }() // Flush the minifier buffer
		w = mw
	}

	if err := r.Layout.Execute(w, data); err != nil {
		log.Printf("❌ Failed to render layout for %s: %v\n", path, err)
	}
}

func (r *Renderer) RenderIndex(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("❌ Failed to create file %s: %v\n", path, err)
		return
	}
	defer func() { _ = f.Close() }()

	var w io.Writer = f

	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	// Use dedicated index template if available, otherwise fall back to layout
	var errExec error
	if r.Index != nil {
		errExec = r.Index.Execute(w, data)
	} else {
		errExec = r.Layout.Execute(w, data)
	}
	if errExec != nil {
		log.Printf("❌ Failed to render index for %s: %v\n", path, errExec)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	if r.Graph == nil {
		return
	}
	data.Assets = r.Assets

	f, err := os.Create(path)
	if err != nil {
		log.Printf("❌ Failed to create file %s: %v\n", path, err)
		return
	}
	defer func() { _ = f.Close() }()

	var w io.Writer = f
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	if err := r.Graph.Execute(w, data); err != nil {
		log.Printf("❌ Failed to render graph for %s: %v\n", path, err)
	}
}

func (r *Renderer) Render404(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("❌ Failed to create file %s: %v\n", path, err)
		return
	}
	defer func() { _ = f.Close() }()

	var w io.Writer = f

	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	// Use dedicated 404 template if available, otherwise fall back to layout
	var errExec error
	if r.NotFound != nil {
		errExec = r.NotFound.Execute(w, data)
	} else {
		errExec = r.Layout.Execute(w, data)
	}
	if errExec != nil {
		log.Printf("❌ Failed to render 404 for %s: %v\n", path, errExec)
	}
}
