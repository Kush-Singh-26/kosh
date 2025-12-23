// Handles template loading and file creation
package renderer

import (
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"

	"my-ssg/builder/models"
)

type Renderer struct {
	Layout  *template.Template
	Graph   *template.Template
	LayoutCSS template.CSS
	ThemeCSS  template.CSS
}

func New() *Renderer {
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
	}
}

func (r *Renderer) RenderPage(path string, data models.PageData) {
	// Inject CSS
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	os.MkdirAll(filepath.Dir(path), 0755)
	f, _ := os.Create(path)
	defer f.Close()
	r.Layout.Execute(f, data)
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	if r.Graph == nil {
		return
	}
	// Inject CSS
	data.LayoutCSS = r.LayoutCSS
	data.ThemeCSS = r.ThemeCSS

	f, _ := os.Create(path)
	defer f.Close()
	r.Graph.Execute(f, data)
}