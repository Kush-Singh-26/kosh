//go:build !wasm

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
	Minify         bool
	Sink           fspkg.ArtifactSink
	SourceFs       afero.Fs
	renderedFiles  sync.Map // path string -> struct{}{}
	logger         *slog.Logger
	templateDir    string
	layoutsDir     string
	mu             sync.RWMutex // protects template pointers and logger
	devMode        bool
	renderErrors   []renderError
	errMu          sync.Mutex // protects renderErrors
	assetCache     sync.Map   // cacheKey string -> map[string]string
	Minifier       *minify.M
	fragmentCache  sync.Map // context string -> template.HTML
	Cache          models.FragmentCache
}

// Options configures a Renderer instance.
type Options struct {
	SourceFs    afero.Fs
	Compress    bool
	Minify      bool
	Sink        fspkg.ArtifactSink
	TemplateDir string
	LayoutsDir  string
	DevMode     bool
	Logger      *slog.Logger
	Cache       models.FragmentCache
}

// New creates a Renderer with default filesystem settings.
func New(opts Options) *Renderer {
	if opts.SourceFs == nil {
		opts.SourceFs = afero.NewOsFs()
	}
	return NewWithFs(opts)
}

// NewWithFs creates a Renderer using the provided filesystem.
func NewWithFs(opts Options) *Renderer {
	r := &Renderer{
		Compress:    opts.Compress,
		Minify:      opts.Minify,
		Sink:        opts.Sink,
		SourceFs:    opts.SourceFs,
		logger:      opts.Logger,
		templateDir: opts.TemplateDir,
		layoutsDir:  opts.LayoutsDir,
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
	r.fragmentCache.Range(func(key, _ any) bool {
		r.fragmentCache.Delete(key)
		return true
	})
}

// ReloadTemplates reloads templates from disk or cache.
func (r *Renderer) ReloadTemplates() {
	tc := getGlobalCache(r.templateDir, r.layoutsDir, r.devMode)

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

	// In-memory cache lookup: skip in dev mode to always use fresh fragment data
	// This prevents stale navbar/footer from broken builds affecting new renders
	if val, ok := r.fragmentCache.Load(cacheKey); ok && !data.IsCleanBuild && !r.devMode {
		return val.(template.HTML), nil
	}

	// Persistent cache lookup: skip if this is a clean build to ensure branding/config updates
	// Also skip in dev mode to avoid stale fragments from prior broken builds
	if r.Cache != nil && !data.IsCleanBuild && !r.devMode {
		if cached, err := r.Cache.GetFragment(cacheKey); err == nil && len(cached) > 0 {
			html := template.HTML(string(cached))
			r.fragmentCache.Store(cacheKey, html)
			return html, nil
		}
	}

	r.mu.RLock()
	parentTmpl := r.Layout
	r.mu.RUnlock()

	if parentTmpl == nil {
		return "", fmt.Errorf("no template available for fragment rendering")
	}

	tmpl := parentTmpl.Lookup(blockName)
	if tmpl == nil {
		return "", fmt.Errorf("template block %s not found", blockName)
	}

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

	// We prepare assets here to ensure path relativization and site data are available,
	// but we call PrepareAssets instead of PreparePageData to avoid infinite recursion
	// with global fragment pre-rendering.
	r.PrepareAssets(&data)

	// Always execute the block from the parent template set using ExecuteTemplate.
	// This ensures that ONLY the named block is rendered, avoiding accidental
	// inheritance of the full "base.html" document which happens if we call
	// Execute() on the block template directly.
	if err := parentTmpl.ExecuteTemplate(buf, blockName, data); err != nil {
		return "", err
	}

	// Persist fragment for cross-build reuse using raw bytes from buffer to avoid extra allocation
	// Skip persisting in dev mode to avoid polluting both caches
	if r.Cache != nil && !r.devMode {
		if err := r.Cache.StoreFragment(cacheKey, buf.Bytes()); err != nil {
			r.logger.Debug("Failed to persist fragment", "key", cacheKey, "error", err)
		}
	}

	html := template.HTML(buf.String())
	// Only store to in-memory cache if not in dev mode to avoid stale data on next render
	if !r.devMode {
		r.fragmentCache.Store(cacheKey, html)
	}

	return html, nil
}

func (r *Renderer) applyTemplateCache(tc *templateCache) {
	funcMap := templateFuncMap()

	applyFuncs := func(t *template.Template) *template.Template {
		if t != nil {
			return t.Funcs(funcMap)
		}
		return t
	}

	r.mu.Lock()
	r.Layout = applyFuncs(tc.templates["layout"])
	r.Index = applyFuncs(tc.templates["index"])
	r.Home = applyFuncs(tc.templates["home"])
	r.Graph = applyFuncs(tc.templates["graph"])
	r.NotFound = applyFuncs(tc.templates["404"])
	r.mu.Unlock()
}

func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"lower":      strings.ToLower,
		"hasPrefix":  strings.HasPrefix,
		"replace":    replaceFunc,
		"trimPrefix": strings.TrimPrefix,
		"relativize": relativizeFunc,
		"now":        time.Now,
		"urlEscape":  url.PathEscape,
		"slugify":    timeutil.Slugify,
		"dateFormat": dateFormatFunc,
		"jsonify":    jsonifyFunc,
		"default":    defaultFunc,
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"add": func(a, b int) int {
			return a + b
		},
	}
}

func replaceFunc(from, to, input string) string {
	return strings.ReplaceAll(input, from, to)
}

func relativizeFunc(baseURL, prefix, link string) string {
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
}

func dateFormatFunc(layout string, value any) string {
	if value == nil {
		return ""
	}
	if dateTime, ok := value.(time.Time); ok {
		return dateTime.UTC().Format(layout)
	}
	if dateStr, ok := value.(string); ok {
		if dateTime, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC); err == nil {
			return dateTime.Format(layout)
		}
		if dateTime, err := time.ParseInLocation("2006-01-02 15:04:05 -0700 MST", dateStr, time.UTC); err == nil {
			return dateTime.Format(layout)
		}
		if dateTime, err := time.ParseInLocation("2006-01-02 15:04:05 -0700", dateStr, time.UTC); err == nil {
			return dateTime.Format(layout)
		}
		return dateStr
	}
	return fmt.Sprintf("%v", value)
}

func jsonifyFunc(value any) (template.JS, error) {
	jsonBytes, err := json.Marshal(value)
	return template.JS(jsonBytes), err
}

func defaultFunc(defaultValue, value any) any {
	if value == nil {
		return defaultValue
	}
	if s, ok := value.(string); ok && s == "" {
		return defaultValue
	}
	return value
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

	eg := new(errgroup.Group)
	r.submitSlotTemplateTasks(eg, tc, baseTmpl, &mu, &layoutTmpl, &indexTmpl, &homeTmpl, &notFoundTmpl)

	eg.Go(func() error {
		tmpl, err := r.loadGraphTemplate(tc, baseTmpl)
		if err == nil {
			mu.Lock()
			graphTmpl = tmpl
			mu.Unlock()
		}
		return err
	})

	if err := eg.Wait(); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return layoutTmpl, indexTmpl, homeTmpl, graphTmpl, notFoundTmpl, nil
}

func (r *Renderer) submitSlotTemplateTasks(eg *errgroup.Group, tc *templateCache, baseTmpl *template.Template, mu *sync.Mutex, layout, index, home, notFound **template.Template) {
	tasks := []struct {
		name     string
		fileName string
		target   **template.Template
	}{
		{"layout", "layout.html", layout},
		{"index", "index.html", index},
		{"home", "home.html", home},
		{"404", "404.html", notFound},
	}

	for _, task := range tasks {
		t := task
		eg.Go(func() error {
			tmpl, err := r.loadSlotTmpl(tc, baseTmpl, mu, t.name, t.fileName)
			if err != nil {
				return r.handleTemplateLoadError(t.name, err)
			}
			mu.Lock()
			*t.target = tmpl
			mu.Unlock()
			return nil
		})
	}
}

func (r *Renderer) handleTemplateLoadError(name string, err error) error {
	if name == "layout" {
		return fmt.Errorf("failed to read layout template: %w", err)
	}
	if name == "home" {
		r.logger.Debug("Home template not found (optional)", "dir", r.templateDir)
	} else {
		r.logger.Warn(fmt.Sprintf("%s template not found, falling back to layout", name), "dir", r.templateDir)
	}
	return nil
}

func (r *Renderer) loadGraphTemplate(tc *templateCache, baseTmpl *template.Template) (*template.Template, error) {
	funcMap := templateFuncMap()

	t, err := baseTmpl.Clone()
	if err != nil {
		return nil, err
	}

	t = t.Funcs(funcMap)

	if _, err := r.loadPartials(t); err != nil {
		return nil, err
	}
	tc.setTemplate("graph", t, time.Now(), []byte(base.BaseTemplate))
	return t, nil
}

func (r *Renderer) loadSlotTmpl(tc *templateCache, baseTmpl *template.Template, mu *sync.Mutex, name, fileName string) (*template.Template, error) {
	funcMap := templateFuncMap()

	// 1. Check Site Layouts
	path := filepath.Join(r.layoutsDir, fileName)
	content, err := afero.ReadFile(r.SourceFs, path)
	if err != nil {
		// 2. Fallback to Theme
		path = filepath.Join(r.templateDir, fileName)
		content, err = afero.ReadFile(r.SourceFs, path)
		if err != nil {
			return nil, err
		}
	}

	// Clone base and parse theme slot on top
	t, err := baseTmpl.Clone()
	if err != nil {
		return nil, err
	}

	t = t.Funcs(funcMap)

	// Load partials from both site and theme
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

// loadPartials discovers and parses all files under templates/partials/ into t.
// Partials are registered as named templates "partials/<filename>" on t.
// loadPartials discovers and parses all files under partials/ into t.
// It merges partials from both the site layouts directory and the theme directory,
// with site partials taking precedence.
func (r *Renderer) loadPartials(t *template.Template) (int, error) {
	var (
		count int32
		mu    sync.Mutex
	)

	loadFrom := func(dir string) error {
		partialsDir := filepath.Join(dir, "partials")
		exists, err := afero.DirExists(r.SourceFs, partialsDir)
		if err != nil || !exists {
			return nil
		}

		return fspkg.ParallelWalk(fspkg.WalkOptions{
			Ctx:      context.Background(),
			SourceFs: r.SourceFs,
			Root:     partialsDir,
			WalkFn: func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".html") {
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
	}

	// 1. Load from Theme (Base)
	if err := loadFrom(r.templateDir); err != nil {
		return 0, err
	}

	// 2. Load from Site (Overrides)
	if err := loadFrom(r.layoutsDir); err != nil {
		return int(count), err
	}

	return int(count), nil
}
