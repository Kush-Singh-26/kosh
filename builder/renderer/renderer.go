// Handles template loading and file creation
package renderer

import (
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"my-ssg/builder/models"
	"my-ssg/builder/utils"
)

// templateCache stores parsed templates with their modification times
type templateCache struct {
	templates   map[string]*template.Template
	mtimes      map[string]time.Time
	templateDir string
	mu          sync.RWMutex
}

var (
	globalCache     *templateCache
	globalCacheOnce sync.Once
)

// getGlobalCache returns the singleton template cache instance
func getGlobalCache(templateDir string) *templateCache {
	globalCacheOnce.Do(func() {
		globalCache = &templateCache{
			templates:   make(map[string]*template.Template),
			mtimes:      make(map[string]time.Time),
			templateDir: templateDir,
		}
	})
	return globalCache
}

// hasTemplatesChanged checks if any template files have been modified since last cache
func (tc *templateCache) hasTemplatesChanged() bool {
	templateFiles := []string{"layout.html", "index.html", "graph.html", "404.html"}

	for _, fname := range templateFiles {
		path := filepath.Join(tc.templateDir, fname)
		info, err := os.Stat(path)
		if err != nil {
			continue // File might not exist, skip
		}

		cachedMtime, exists := tc.mtimes[fname]
		if !exists || info.ModTime().After(cachedMtime) {
			return true
		}
	}

	return false
}

// setTemplate caches a template with its modification time
func (tc *templateCache) setTemplate(name string, tmpl *template.Template, mtime time.Time) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.templates[name] = tmpl
	tc.mtimes[name] = mtime
}

type Renderer struct {
	Layout      *template.Template
	Index       *template.Template
	Graph       *template.Template
	NotFound    *template.Template
	Assets      map[string]string
	Compress    bool
	DestFs      afero.Fs
	RenderedMu  sync.Mutex
	RenderedSet map[string]bool
}

func New(compress bool, destFs afero.Fs, templateDir string) *Renderer {
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"now":       time.Now,
	}

	// Get or create the global template cache
	tc := getGlobalCache(templateDir)

	// Check if we can use cached templates
	tc.mu.RLock()
	cacheValid := len(tc.templates) > 0 && !tc.hasTemplatesChanged()
	if cacheValid {
		// Return cached renderer
		r := &Renderer{
			Layout:      tc.templates["layout"],
			Index:       tc.templates["index"],
			Graph:       tc.templates["graph"],
			NotFound:    tc.templates["404"],
			Compress:    compress,
			DestFs:      destFs,
			RenderedSet: make(map[string]bool),
		}
		tc.mu.RUnlock()
		return r
	}
	tc.mu.RUnlock()

	// Templates are read from OS filesystem (Source code)
	// We could abstract this too, but templates are usually static source.
	// Assuming running from root.

	// Load layout template
	layoutPath := filepath.Join(templateDir, "layout.html")
	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFiles(layoutPath)
	if err != nil {
		log.Fatal(err)
	}
	layoutInfo, _ := os.Stat(layoutPath)
	if layoutInfo != nil {
		tc.setTemplate("layout", tmpl, layoutInfo.ModTime())
	}

	// Load index template
	indexPath := filepath.Join(templateDir, "index.html")
	indexTmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles(indexPath)
	if err != nil {
		log.Printf("⚠️  Index template not found in %s, will use layout.html for index. (%v)\n", templateDir, err)
		indexTmpl = nil
	} else {
		indexInfo, _ := os.Stat(indexPath)
		if indexInfo != nil {
			tc.setTemplate("index", indexTmpl, indexInfo.ModTime())
		}
	}

	// Load graph template
	graphPath := filepath.Join(templateDir, "graph.html")
	graphTmpl, err := template.ParseFiles(graphPath)
	if err != nil {
		log.Printf("⚠️  Graph template not found in %s, skipping graph page. (%v)\n", templateDir, err)
	} else {
		graphInfo, _ := os.Stat(graphPath)
		if graphInfo != nil {
			tc.setTemplate("graph", graphTmpl, graphInfo.ModTime())
		}
	}

	// Load 404 template
	notFoundPath := filepath.Join(templateDir, "404.html")
	notFoundTmpl, err := template.New("404.html").Funcs(funcMap).ParseFiles(notFoundPath)
	if err != nil {
		log.Printf("⚠️  404 template not found in %s, will use layout.html for 404 page. (%v)\n", templateDir, err)
		notFoundTmpl = nil
	} else {
		notFoundInfo, _ := os.Stat(notFoundPath)
		if notFoundInfo != nil {
			tc.setTemplate("404", notFoundTmpl, notFoundInfo.ModTime())
		}
	}

	return &Renderer{
		Layout:      tmpl,
		Index:       indexTmpl,
		Graph:       graphTmpl,
		NotFound:    notFoundTmpl,
		Compress:    compress,
		DestFs:      destFs,
		RenderedSet: make(map[string]bool),
	}
}

func (r *Renderer) RegisterFile(path string) {
	r.RenderedMu.Lock()
	defer r.RenderedMu.Unlock()
	// Normalize path to forward slashes for consistency
	r.RenderedSet[filepath.ToSlash(path)] = true
}

func (r *Renderer) GetRenderedFiles() map[string]bool {
	r.RenderedMu.Lock()
	defer r.RenderedMu.Unlock()
	return r.RenderedSet
}

func (r *Renderer) ClearRenderedFiles() {
	r.RenderedMu.Lock()
	defer r.RenderedMu.Unlock()
	r.RenderedSet = make(map[string]bool)
}

func (r *Renderer) SetAssets(assets map[string]string) {

	r.Assets = assets
}

func (r *Renderer) RenderPage(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}

	f, err := r.DestFs.Create(path)
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
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) RenderIndex(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}
	f, err := r.DestFs.Create(path)
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
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	if r.Graph == nil {
		return
	}
	data.Assets = r.Assets

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}

	f, err := r.DestFs.Create(path)
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
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) Render404(path string, data models.PageData) {
	// Inject Assets
	data.Assets = r.Assets

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("❌ Failed to create directory for %s: %v\n", path, err)
		return
	}
	f, err := r.DestFs.Create(path)
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
	} else {
		r.RegisterFile(path)
	}
}
