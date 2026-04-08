package renderer

import (
	"encoding/json"
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

	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// Renderer loads templates and renders site pages.
type Renderer struct {
	Layout         *template.Template
	Index          *template.Template
	Graph          *template.Template
	NotFound       *template.Template
	assetsSnapshot atomic.Pointer[map[string]string]
	Compress       bool
	Sink           fspkg.ArtifactSink
	SourceFs       afero.Fs
	renderedFiles  sync.Map
	logger         *slog.Logger
	templateDir    string
	mu             sync.RWMutex // protects template pointers and logger
	devMode        bool
	renderErrors   []renderError
	errMu          sync.Mutex // protects renderErrors
	assetCache     sync.Map
	Minifier       *minify.M
}

// RendererOptions configures a Renderer instance.
type RendererOptions struct {
	SourceFs    afero.Fs
	Compress    bool
	Sink        fspkg.ArtifactSink
	TemplateDir string
	DevMode     bool
	Logger      *slog.Logger
}

// New creates a Renderer with default filesystem settings.
func New(opts RendererOptions) *Renderer {
	if opts.SourceFs == nil {
		opts.SourceFs = afero.NewOsFs()
	}
	return NewWithFs(opts)
}

// NewWithFs creates a Renderer using the provided filesystem.
func NewWithFs(opts RendererOptions) *Renderer {
	r := &Renderer{
		Compress:    opts.Compress,
		Sink:        opts.Sink,
		SourceFs:    opts.SourceFs,
		logger:      opts.Logger,
		templateDir: opts.TemplateDir,
		devMode:     opts.DevMode,
		Minifier:    koshMinify.GetHTMLMinifier(),
	}
	r.ReloadTemplates()
	return r
}

// SetSink swaps the output sink used for rendered pages.
func (r *Renderer) SetSink(sink fspkg.ArtifactSink) {
	r.Sink = sink
}

// SetLogger updates the logger used for renderer diagnostics.
func (r *Renderer) SetLogger(l *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = l
}

// Has404Template returns true if the 404.html template was successfully loaded from the theme.
// This allows the build pipeline to render a 404 page even without a content/404.md file.
func (r *Renderer) Has404Template() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.NotFound != nil
}

// ReloadTemplates reloads templates from disk or cache.
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
		"dateFormat": func(layout string, v any) string {
			if v == nil {
				return ""
			}
			if t, ok := v.(time.Time); ok {
				return t.Format(layout)
			}
			if s, ok := v.(string); ok {
				if t, err := time.Parse("2006-01-02", s); err == nil {
					return t.Format(layout)
				}
				if t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", s); err == nil {
					return t.Format(layout)
				}
				if t, err := time.Parse("2006-01-02 15:04:05 -0700", s); err == nil {
					return t.Format(layout)
				}
				return s
			}
			return fmt.Sprintf("%v", v)
		},
		"jsonify": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	}

	var (
		layoutTmpl, indexTmpl, graphTmpl, notFoundTmpl *template.Template
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

	if err := g.Wait(); err != nil {
		r.logger.Error("Template parsing failed", "error", err)
		os.Exit(1)
	}

	r.mu.Lock()
	r.Layout = layoutTmpl
	r.Index = indexTmpl
	r.Graph = graphTmpl
	r.NotFound = notFoundTmpl
	r.mu.Unlock()
}
