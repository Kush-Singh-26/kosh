// Handles template loading and file creation
package renderer

import (
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

type Renderer struct {
	Layout      *template.Template
	Index       *template.Template
	Graph       *template.Template
	NotFound    *template.Template
	LayoutCSS   template.CSS
	ThemeCSS    template.CSS
	Compress    bool
}

func New(compress bool) *Renderer {
	funcMap := template.FuncMap{"lower": strings.ToLower}
	
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

	layoutBytes, _ := os.ReadFile("static/css/layout.css")
	themeBytes, _ := os.ReadFile("static/css/theme.css")

	return &Renderer{
		Layout:    tmpl,
		Index:     indexTmpl,
		Graph:     graphTmpl,
		NotFound:  notFoundTmpl,
		LayoutCSS: template.CSS(layoutBytes),
		ThemeCSS:  template.CSS(themeBytes),
		Compress:  compress,
	}
}

func (r *Renderer) RenderPage(path string, data models.PageData) {
	// Inject CSS
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()

	var w io.Writer = f
	
	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer mw.Close() // Flush the minifier buffer
		w = mw
	}

	r.Layout.Execute(w, data)
}

func (r *Renderer) RenderIndex(path string, data models.PageData) {
	// Inject CSS
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()

	var w io.Writer = f
	
	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer mw.Close()
		w = mw
	}

	// Use dedicated index template if available, otherwise fall back to layout
	if r.Index != nil {
		r.Index.Execute(w, data)
	} else {
		r.Layout.Execute(w, data)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	if r.Graph == nil {
		return
	}
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	f, _ := os.Create(path)
	defer f.Close()

	var w io.Writer = f
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer mw.Close()
		w = mw
	}

	r.Graph.Execute(w, data)
}

func (r *Renderer) Render404(path string, data models.PageData) {
	// Inject CSS
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()

	var w io.Writer = f
	
	// Minify HTML if enabled
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", f)
		defer mw.Close()
		w = mw
	}

	// Use dedicated 404 template if available, otherwise fall back to layout
	if r.NotFound != nil {
		r.NotFound.Execute(w, data)
	} else {
		r.Layout.Execute(w, data)
	}
}