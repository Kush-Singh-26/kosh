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
	renderedFiles  sync.Map // path string -> struct{}{}
	logger         *slog.Logger
	templateDir    string
	mu             sync.RWMutex // protects template pointers and logger
	devMode        bool
	renderErrors   []renderError
	errMu          sync.Mutex // protects renderErrors
	assetCache     sync.Map   // cacheKey string -> map[string]string
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
func (r *Renderer) SetLogger(logger *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = logger
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
		r.applyTemplateCache(tc)
		return
	}

	funcMap := templateFuncMap()
	layoutTmpl, indexTmpl, graphTmpl, notFoundTmpl, err := r.loadTemplates(tc, funcMap)
	if err != nil {
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

func (r *Renderer) applyTemplateCache(tc *templateCache) {
	r.mu.Lock()
	r.Layout = tc.templates["layout"]
	r.Index = tc.templates["index"]
	r.Graph = tc.templates["graph"]
	r.NotFound = tc.templates["404"]
	r.mu.Unlock()
}

func templateFuncMap() template.FuncMap {
	return template.FuncMap{
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

			if strings.HasPrefix(link, "http") || strings.HasPrefix(link, "//") || strings.HasPrefix(link, "data:") {
				return link
			}

			isHome := link == "/"
			if link[0] != '/' {
				link = "/" + link
			}

			if baseURL != "" {
				return filepath.ToSlash(strings.TrimSuffix(baseURL, "/") + link)
			}

			if prefix == "" || prefix == "." || prefix == "./" {
				if isHome {
					return "index.html"
				}
				return filepath.ToSlash(link[1:])
			}

			if isHome {
				return filepath.ToSlash(prefix + "index.html")
			}
			return filepath.ToSlash(prefix + link[1:])
		},
		"now":       time.Now,
		"urlEscape": url.PathEscape,
		"slugify":   timeutil.Slugify,
		"dateFormat": func(layout string, value any) string {
			if value == nil {
				return ""
			}
			if dateTime, ok := value.(time.Time); ok {
				return dateTime.Format(layout)
			}
			if dateStr, ok := value.(string); ok {
				if dateTime, err := time.Parse("2006-01-02", dateStr); err == nil {
					return dateTime.Format(layout)
				}
				if dateTime, err := time.Parse("2006-01-02 15:04:05 -0700 MST", dateStr); err == nil {
					return dateTime.Format(layout)
				}
				if dateTime, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr); err == nil {
					return dateTime.Format(layout)
				}
				return dateStr
			}
			return fmt.Sprintf("%v", value)
		},
		"jsonify": func(value any) (string, error) {
			jsonBytes, err := json.Marshal(value)
			return string(jsonBytes), err
		},
	}
}

func (r *Renderer) loadTemplates(tc *templateCache, funcMap template.FuncMap) (*template.Template, *template.Template, *template.Template, *template.Template, error) {
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

	eg := new(errgroup.Group)

	eg.Go(func() error {
		tmpl, err := loadTmpl("layout", "layout.html")
		if err != nil {
			return fmt.Errorf("failed to read layout template: %w", err)
		}
		mu.Lock()
		layoutTmpl = tmpl
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadTmpl("index", "index.html")
		if err != nil {
			r.logger.Warn("Index template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		indexTmpl = tmpl
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadTmpl("graph", "graph.html")
		if err != nil {
			r.logger.Warn("Graph template not found, skipping graph page", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		graphTmpl = tmpl
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadTmpl("404", "404.html")
		if err != nil {
			r.logger.Warn("404 template not found, falling back to layout", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		notFoundTmpl = tmpl
		mu.Unlock()
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, nil, nil, nil, err
	}
	return layoutTmpl, indexTmpl, graphTmpl, notFoundTmpl, nil
}
