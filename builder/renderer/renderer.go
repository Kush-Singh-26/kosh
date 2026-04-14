package renderer

import (
	"context"
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

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/renderer/base"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// Renderer loads templates and renders site pages.
type Renderer struct {
	Layout         *template.Template
	Index          *template.Template
	Home           *template.Template
	Graph          *template.Template
	NotFound       *template.Template
	baseTemplate   *template.Template
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
	fragmentCache sync.Map // context string -> template.HTML
	Cache         models.FragmentCache
}

// RendererOptions configures a Renderer instance.
type RendererOptions struct {
	SourceFs    afero.Fs
	Compress    bool
	Sink        fspkg.ArtifactSink
	TemplateDir string
	DevMode     bool
	Logger      *slog.Logger
	Cache       models.FragmentCache
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
		Cache:       opts.Cache,
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

// ClearFragments empties the pre-rendered fragment cache.
func (r *Renderer) ClearFragments() {
	r.fragmentCache.Range(func(key, value any) bool {
		r.fragmentCache.Delete(key)
		return true
	})
}

// ReloadTemplates reloads templates from disk or cache.
func (r *Renderer) ReloadTemplates() {
	tc := getGlobalCache(r.templateDir, r.devMode)

	tc.mu.RLock()
	hasTemplates := len(tc.templates) > 0
	tc.mu.RUnlock()

	// Do not hold any locks when calling hasTemplatesChanged() to avoid deadlock
	cacheValid := hasTemplates && !tc.hasTemplatesChanged(r.SourceFs)

	if cacheValid {
		r.applyTemplateCache(tc)
		return
	}

	funcMap := templateFuncMap()
	layoutTmpl, indexTmpl, homeTmpl, graphTmpl, notFoundTmpl, err := r.loadTemplates(tc, funcMap)
	if err != nil {
		r.logger.Error("Template parsing failed", "error", err)
		os.Exit(1)
	}

	r.mu.Lock()
	r.Layout = layoutTmpl
	r.Index = indexTmpl
	r.Home = homeTmpl
	r.Graph = graphTmpl
	r.NotFound = notFoundTmpl
	r.baseTemplate = layoutTmpl // Use layout as base for fragment rendering
	r.mu.Unlock()

	r.ClearFragments()
}

// RenderFragment renders a specific named template block into the cache.
func (r *Renderer) RenderFragment(context string, blockName string, data models.PageData) (template.HTML, error) {
	// Cache key includes block name, context and relative prefix to ensure path safety
	cacheKey := fmt.Sprintf("%s:%s:%s", blockName, context, data.RelativePrefix)

	if val, ok := r.fragmentCache.Load(cacheKey); ok && !data.IsCleanBuild {
		return val.(template.HTML), nil
	}

	// Persistent cache lookup: skip if this is a clean build to ensure branding/config updates
	if r.Cache != nil && !data.IsCleanBuild {
		if cached, err := r.Cache.GetFragment(cacheKey); err == nil && cached != "" {
			html := template.HTML(cached)
			r.fragmentCache.Store(cacheKey, html)
			return html, nil
		}
	}

	r.mu.RLock()
	tmpl := r.Layout
	r.mu.RUnlock()

	if tmpl == nil {
		return "", fmt.Errorf("no template available for fragment rendering")
	}

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

	// We prepare assets here to ensure path relativization and site data are available,
	// but we call PrepareAssets instead of PreparePageData to avoid infinite recursion
	// with global fragment pre-rendering.
	r.PrepareAssets(&data)

	if err := tmpl.ExecuteTemplate(buf, blockName, data); err != nil {
		return "", err
	}

	html := template.HTML(buf.String())
	r.fragmentCache.Store(cacheKey, html)

	// Persist fragment for cross-build reuse
	if r.Cache != nil {
		if err := r.Cache.StoreFragment(cacheKey, string(html)); err != nil {
			r.logger.Debug("Failed to persist fragment", "key", cacheKey, "error", err)
		}
	}

	return html, nil
}

func (r *Renderer) applyTemplateCache(tc *templateCache) {
	r.mu.Lock()
	r.Layout = tc.templates["layout"]
	r.Index = tc.templates["index"]
	r.Home = tc.templates["home"]
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
				slog.Debug("relativize called", "baseURL", baseURL, "link", link)
				if strings.HasPrefix(link, baseURL) {
					return filepath.ToSlash(link)
				}
				// Also check if it starts with the path part of baseURL
				if u, err := url.Parse(baseURL); err == nil && u.Path != "" {
					if strings.HasPrefix(link, u.Path) {
						return filepath.ToSlash(link)
					}
				}
				return filepath.ToSlash(strings.TrimSuffix(baseURL, "/") + link)
			}

			// Handle root-relative prefix specifically
			if prefix == "/" {
				return filepath.ToSlash(link)
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
		"default": func(defaultValue, value any) any {
			if value == nil {
				return defaultValue
			}
			if s, ok := value.(string); ok && s == "" {
				return defaultValue
			}
			return value
		},
		"add": func(a, b int) int {
			return a + b
		},
	}
}

func (r *Renderer) loadTemplates(tc *templateCache, funcMap template.FuncMap) (*template.Template, *template.Template, *template.Template, *template.Template, *template.Template, error) {
	var (
		layoutTmpl, indexTmpl, homeTmpl, graphTmpl, notFoundTmpl *template.Template
		mu                                                       sync.Mutex
	)

	baseTmpl, err := template.New("base.html").Funcs(funcMap).Parse(base.BaseTemplate)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse embedded base template: %w", err)
	}

	loadSlotTmpl := func(name, fileName string) (*template.Template, error) {
		path := filepath.Join(r.templateDir, fileName)
		content, err := afero.ReadFile(r.SourceFs, path)
		if err != nil {
			return nil, err
		}

		// Clone base and parse theme slot on top
		t, err := baseTmpl.Clone()
		if err != nil {
			return nil, err
		}

		// Load partials into the clone
		if _, err := r.loadPartials(t); err != nil {
			return nil, err
		}

		if _, err := t.New(fileName).Parse(string(content)); err != nil {
			return nil, err
		}

		info, _ := r.SourceFs.Stat(path)
		mu.Lock()
		if info != nil {
			tc.setTemplate(name, t, info.ModTime(), content)
		}
		mu.Unlock()
		return t, nil
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		tmpl, err := loadSlotTmpl("layout", "layout.html")
		if err != nil {
			return fmt.Errorf("failed to read layout template: %w", err)
		}
		mu.Lock()
		layoutTmpl = tmpl
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadSlotTmpl("index", "index.html")
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
		// Graph is now base-only clone
		t, err := baseTmpl.Clone()
		if err != nil {
			return err
		}
		// Load partials into the clone
		if _, err := r.loadPartials(t); err != nil {
			return err
		}
		mu.Lock()
		graphTmpl = t
		tc.setTemplate("graph", t, time.Now(), []byte(base.BaseTemplate))
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadSlotTmpl("home", "home.html")
		if err != nil {
			r.logger.Debug("Home template not found (optional)", "dir", r.templateDir)
			return nil
		}
		mu.Lock()
		homeTmpl = tmpl
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		tmpl, err := loadSlotTmpl("404", "404.html")
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
		return nil, nil, nil, nil, nil, err
	}
	return layoutTmpl, indexTmpl, homeTmpl, graphTmpl, notFoundTmpl, nil
}

// loadPartials discovers and parses all files under templates/partials/ into t.
// Partials are registered as named templates "partials/<filename>" on t.
func (r *Renderer) loadPartials(t *template.Template) (int, error) {
	partialsDir := filepath.Join(r.templateDir, "partials")
	exists, err := afero.DirExists(r.SourceFs, partialsDir)
	if err != nil || !exists {
		return 0, nil // No partials directory is fine
	}

	var (
		count int32
		mu    sync.Mutex
	)

	walkingCtx := context.Background() // Renderer doesn't usually take ctx here, but ParallelWalk requires it.

	walkErr := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:      walkingCtx,
		SourceFs: r.SourceFs,
		Root:     partialsDir,
		WalkFn: func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".html") {
				return nil
			}

			relPath, err := filepath.Rel(partialsDir, path)
			if err != nil {
				return err
			}

			content, err := afero.ReadFile(r.SourceFs, path)
			if err != nil {
				return err
			}

			// Register as "partials/filename.html"
			// Using filepath.ToSlash for consistent template names across OSes
			name := "partials/" + filepath.ToSlash(relPath)

			mu.Lock()
			defer mu.Unlock()

			if _, err := t.New(name).Parse(string(content)); err != nil {
				return fmt.Errorf("failed to parse partial %s: %w", name, err)
			}

			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	if walkErr != nil {
		return int(atomic.LoadInt32(&count)), fmt.Errorf("failed to walk partials directory: %w", walkErr)
	}

	return int(atomic.LoadInt32(&count)), nil
}
