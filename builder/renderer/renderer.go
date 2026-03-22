package renderer

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"
	"github.com/tdewolff/minify/v2"
	"golang.org/x/sync/errgroup"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	koshMinify "github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"

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
	mu               sync.RWMutex
	devMode          bool
	renderErrors     []renderError
	errMu            sync.Mutex
	assetCache       sync.Map
	Minifier         *minify.M
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
		Minifier:    koshMinify.GetHTMLMinifier(),
	}
	r.ReloadTemplates()
	return r
}

func (r *Renderer) SetSink(sink fspkg.ArtifactSink) {
	r.Sink = sink
}

// Has404Template returns true if the 404.html template was successfully loaded from the theme.
// This allows the build pipeline to render a 404 page even without a content/404.md file.
func (r *Renderer) Has404Template() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.NotFound != nil
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
				return filepath.ToSlash(strings.TrimSuffix(baseURL, "/") + link)
			}

			// If baseURL is empty, use RelativePrefix
			if prefix == "" || prefix == "." || prefix == "./" {
				if isHome {
					return "index.html"
				}
				return filepath.ToSlash(link[1:]) // Just remove leading slash
			}

			if isHome {
				return filepath.ToSlash(prefix + "index.html")
			}
			return filepath.ToSlash(prefix + link[1:])
		},
		"now":       time.Now,
		"urlEscape": url.PathEscape,
		"slugify":   timeutil.Slugify,
	}

	var (
		layoutTmpl, indexTmpl, graphTmpl, notFoundTmpl *template.Template
		sidebarTmpl                                    *template.Template
		mu                                             sync.Mutex
	)

	loadTmpl := func(name, fileName string) (*template.Template, error) {
		path := filepath.Join(r.templateDir, fileName)
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			return nil, err
		}
		tmpl, err := template.New(fileName).Funcs(funcMap).Parse(string(content))
		if err != nil {
			return nil, err
		}
		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		if info != nil {
			tc.setTemplate(name, tmpl, info.ModTime(), content)
		}
		mu.Unlock()
		return tmpl, nil
	}

	g := new(errgroup.Group)

	g.Go(func() error {
		t, err := loadTmpl("layout", "layout.html")
		if err != nil {
			return fmt.Errorf("failed to read layout template: %w", err)
		}
		mu.Lock()
		layoutTmpl = t
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		t, err := loadTmpl("index", "index.html")
		if err != nil {
			r.logger.Warn("Index template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		indexTmpl = t
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		t, err := loadTmpl("graph", "graph.html")
		if err != nil {
			r.logger.Warn("Graph template not found, skipping graph page", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		graphTmpl = t
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		t, err := loadTmpl("404", "404.html")
		if err != nil {
			r.logger.Warn("404 template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		notFoundTmpl = t
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		t, err := loadTmpl("sidebar", "sidebar.html")
		if err != nil {
			return nil
		}
		mu.Lock()
		sidebarTmpl = t
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

func (r *Renderer) RenderSidebar(tree []*models.TreeNode) template.HTML {
	r.mu.RLock()
	sidebar := r.Sidebar
	r.mu.RUnlock()

	if sidebar == nil {
		return ""
	}

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

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
