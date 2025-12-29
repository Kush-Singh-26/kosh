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
	Layout    *template.Template
	Graph     *template.Template
	LayoutCSS template.CSS
	ThemeCSS  template.CSS
	Compress  bool
}

func New(compress bool) *Renderer {
	funcMap := template.FuncMap{"lower": strings.ToLower}
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFiles("templates/layout.html")
	if err != nil {
		log.Fatal(err)
	}

	graphTmpl, err := template.ParseFiles("templates/graph.html")
	if err != nil {
		log.Printf("⚠️  Graph template not found, skipping graph page. (%v)\n", err)
	}

	layoutBytes, _ := os.ReadFile("static/css/layout.css")
	themeBytes, _ := os.ReadFile("static/css/theme.css")

	return &Renderer{
		Layout:    tmpl,
		Graph:     graphTmpl,
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