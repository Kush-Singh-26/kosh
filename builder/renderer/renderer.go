package renderer

import (
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

)

type Renderer struct {
	Layout           *template.Template
	Index            *template.Template
	Graph            *template.Template
	NotFound         *template.Template
	Sidebar          *template.Template
	Assets           map[string]string
	AssetsMu         sync.RWMutex
	assetsSnapshot   atomic.Pointer[map[string]string]
	Compress         bool
	Sink             fspkg.ArtifactSink
	SourceFs         afero.Fs
	RenderedMu       sync.RWMutex
	RenderedSet      map[string]bool
	renderedSnapshot atomic.Pointer[map[string]bool]
	logger           *slog.Logger
	templateDir      string
	mu               sync.RWMutex // Added for thread-safe template access
	devMode          bool
	renderErrors     []renderError
	errMu            sync.Mutex
	assetCache       sync.Map // Cache for relativized asset maps keyed by depth/prefix
}

type renderError struct {
	msg  string
	path string
	err  error
}

func New(compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer {
	return NewWithFs(afero.NewOsFs(), compress, sink, templateDir, devMode, logger)
}

func NewWithFs(sourceFs afero.Fs, compress bool, sink fspkg.ArtifactSink, templateDir string, devMode bool, logger *slog.Logger) *Renderer {
	r := &Renderer{
		Compress:    compress,
		Sink:        sink,
		SourceFs:    sourceFs,
		RenderedSet: make(map[string]bool),
		logger:      logger,
		templateDir: templateDir,
		devMode:     devMode,
	}
	r.ReloadTemplates()
	return r
}

func (r *Renderer) SetSink(sink fspkg.ArtifactSink) {
	r.Sink = sink
}

func (r *Renderer) ReloadTemplates() {
	tc := getGlobalCache(r.templateDir, r.devMode)

	tc.mu.RLock()
	hasTemplates := len(tc.templates) > 0
	tc.mu.RUnlock()

	// Do not hold any locks when calling hasTemplatesChanged() to avoid deadlock
	cacheValid := hasTemplates && !tc.hasTemplatesChanged()

	if cacheValid {
		r.mu.Lock()
		r.Layout = tc.templates["layout"]
		r.Index = tc.templates["index"]
		r.Graph = tc.templates["graph"]
		r.NotFound = tc.templates["404"]
		r.Sidebar = tc.templates["sidebar"]
		r.mu.Unlock()
		return
	}

	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"replace": func(from, to, input string) string {
			return strings.ReplaceAll(input, from, to)
		},
		"trimPrefix": strings.TrimPrefix,
		"relativize": func(baseURL, prefix, link string) string {
			if len(link) == 0 {
				return link
			}

			// Fast path for absolute URLs
			if strings.HasPrefix(link, "http") || strings.HasPrefix(link, "//") || strings.HasPrefix(link, "data:") {
				return link
			}

			// Clean the link
			isHome := link == "/"
			if link[0] != '/' {
				link = "/" + link
			}

			if baseURL != "" {
				return strings.TrimSuffix(baseURL, "/") + link
			}

			// If baseURL is empty, use RelativePrefix
			if prefix == "" || prefix == "." || prefix == "./" {
				if isHome {
					return "index.html"
				}
				return link[1:] // Just remove leading slash
			}

			if isHome {
				return prefix + "index.html"
			}
			return prefix + link[1:]
		},
		"now":       time.Now,
		"urlEscape": url.PathEscape,
		"slugify":   timeutil.Slugify,
	}

	var (
		layoutTmpl, indexTmpl, graphTmpl, notFoundTmpl *template.Template
		mu                                             sync.Mutex
	)

	g := new(errgroup.Group)

	// Layout (Essential)
	g.Go(func() error {
		path := filepath.Join(r.templateDir, "layout.html")
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			return fmt.Errorf("failed to read layout template: %w", err)
		}
		tmpl, err := template.New("layout.html").Funcs(funcMap).Parse(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse layout template: %w", err)
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		layoutTmpl = tmpl
		if info != nil {
			tc.setTemplate("layout", tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return nil
	})

	// Index
	g.Go(func() error {
		path := filepath.Join(r.templateDir, "index.html")
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			r.logger.Warn("Index template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		tmpl, err := template.New("index.html").Funcs(funcMap).Parse(string(content))
		if err != nil {
			r.logger.Warn("Failed to parse index template", "path", path, "error", err)
			return nil
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		indexTmpl = tmpl
		if info != nil {
			tc.setTemplate("index", tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return nil
	})

	// Graph
	g.Go(func() error {
		path := filepath.Join(r.templateDir, "graph.html")
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			r.logger.Warn("Graph template not found, skipping graph page", "dir", r.templateDir)
			return nil
		}
		tmpl, err := template.New("graph.html").Funcs(funcMap).Parse(string(content))
		if err != nil {
			r.logger.Warn("Failed to parse graph template", "path", path, "error", err)
			return nil
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		graphTmpl = tmpl
		if info != nil {
			tc.setTemplate("graph", tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return nil
	})

	// 404
	g.Go(func() error {
		path := filepath.Join(r.templateDir, "404.html")
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			r.logger.Warn("404 template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		tmpl, err := template.New("404.html").Funcs(funcMap).Parse(string(content))
		if err != nil {
			r.logger.Warn("Failed to parse 404 template", "path", path, "error", err)
			return nil
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		notFoundTmpl = tmpl
		if info != nil {
			tc.setTemplate("404", tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return nil
	})

	// Sidebar (Optional Component)
	var sidebarTmpl *template.Template
	g.Go(func() error {
		path := filepath.Join(r.templateDir, "sidebar.html")
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			return nil // Optional
		}
		tmpl, err := template.New("sidebar.html").Funcs(funcMap).Parse(string(content))
		if err != nil {
			r.logger.Warn("Failed to parse sidebar template", "path", path, "error", err)
			return nil
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		sidebarTmpl = tmpl
		if info != nil {
			tc.setTemplate("sidebar", tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return nil
	})

	if err := g.Wait(); err != nil {
		r.logger.Error("Template parsing failed", "error", err)
		os.Exit(1)
	}

	r.mu.Lock()
	r.Layout = layoutTmpl
	r.Index = indexTmpl
	r.Graph = graphTmpl
	r.NotFound = notFoundTmpl
	r.Sidebar = sidebarTmpl
	r.mu.Unlock()
}

func (r *Renderer) RegisterFile(path string) {
	r.RenderedMu.Lock()
	r.RenderedSet[path] = true
	// Invalidate snapshot so GetRenderedFiles rebuilds it lazily
	r.renderedSnapshot.Store(nil)
	r.RenderedMu.Unlock()
}

func (r *Renderer) GetRenderedFiles() map[string]bool {
	// Fast path: valid snapshot already exists
	if s := r.renderedSnapshot.Load(); s != nil {
		return *s
	}
	// Slow path: build snapshot under lock, then cache it
	r.RenderedMu.Lock()
	snapshot := make(map[string]bool, len(r.RenderedSet))
	maps.Copy(snapshot, r.RenderedSet)
	r.renderedSnapshot.Store(&snapshot)
	r.RenderedMu.Unlock()
	return snapshot
}

func (r *Renderer) ClearRenderedFiles() {
	r.RenderedMu.Lock()
	r.RenderedSet = make(map[string]bool)
	r.renderedSnapshot.Store(nil)
	r.RenderedMu.Unlock()
}

func (r *Renderer) SetAssets(assets map[string]string) {
	r.AssetsMu.Lock()
	r.Assets = assets
	// Create snapshot
	snapshot := make(map[string]string, len(assets))
	maps.Copy(snapshot, assets)
	r.assetsSnapshot.Store(&snapshot)
	// Invalidate relativization cache because assets have changed
	r.assetCache.Range(func(key, value any) bool {
		r.assetCache.Delete(key)
		return true
	})
	r.AssetsMu.Unlock()
}

// PreparePageData performs common optimizations like asset map relativization
func (r *Renderer) PreparePageData(data *models.PageData) {
	if data.Assets == nil {
		data.Assets = r.GetAssets()
	}

	// Optimization: Use cached relativized asset maps to save massive allocation churn
	if len(data.Assets) > 0 {
		cacheKey := data.BaseURL + "|" + data.RelativePrefix
		if cached, ok := r.assetCache.Load(cacheKey); ok {
			data.Assets = cached.(map[string]string)
		} else {
			relativizedAssets := make(map[string]string, len(data.Assets))
			prefix := data.RelativePrefix
			baseURL := data.BaseURL
			for k, v := range data.Assets {
				link := v
				if link[0] != '/' {
					link = "/" + link
				}
				if baseURL != "" {
					relativizedAssets[k] = strings.TrimSuffix(baseURL, "/") + link
				} else if prefix == "" || prefix == "." || prefix == "./" {
					relativizedAssets[k] = link[1:]
				} else {
					relativizedAssets[k] = prefix + link[1:]
				}
			}
			r.assetCache.Store(cacheKey, relativizedAssets)
			data.Assets = relativizedAssets
		}
	}
}

// GetAssets returns a copy of the asset map to prevent accidental mutation
// of the shared global cache state. Maps are reference types in Go, so
// returning the underlying map directly would allow callers to mutate it.
func (r *Renderer) GetAssets() map[string]string {
	s := r.assetsSnapshot.Load()
	if s == nil {
		return make(map[string]string)
	}
	// Return a copy to prevent mutation of shared state
	result := make(map[string]string, len(*s))
	for k, v := range *s {
		result[k] = v
	}
	return result
}

func (r *Renderer) RenderSidebar(tree []*models.TreeNode) template.HTML {
	r.mu.RLock()
	sidebar := r.Sidebar
	r.mu.RUnlock()

	if sidebar == nil {
		return ""
	}

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	// Wrap in a map so we can add other global context if needed
	data := map[string]any{
		"SiteTree": tree,
	}

	if err := sidebar.Execute(buf, data); err != nil {
		r.logger.Error("Failed to render sidebar component", "error", err)
		return ""
	}

	return template.HTML(buf.String())
}

// recordError logs a render error and stores it for later retrieval
func (r *Renderer) recordError(msg string, path string, err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	r.renderErrors = append(r.renderErrors, renderError{msg: msg, path: path, err: err})
	r.logger.Error(msg, "path", path, "error", err)
}

// ConsumeErrors returns all accumulated render errors and clears the error list
func (r *Renderer) ConsumeErrors() []error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if len(r.renderErrors) == 0 {
		return nil
	}
	result := make([]error, len(r.renderErrors))
	for i, e := range r.renderErrors {
		result[i] = fmt.Errorf("%s (path: %s): %w", e.msg, e.path, e.err)
	}
	r.renderErrors = nil // Clear after retrieval
	return result
}
