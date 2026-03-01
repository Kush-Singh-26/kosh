package renderer

import (
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

type Renderer struct {
	Layout      *template.Template
	Index       *template.Template
	Graph       *template.Template
	NotFound    *template.Template
	Assets      map[string]string
	AssetsMu    sync.RWMutex
	Compress    bool
	DestFs      afero.Fs
	RenderedMu  sync.RWMutex
	RenderedSet map[string]bool
	logger      *slog.Logger
}

func New(compress bool, destFs afero.Fs, templateDir string, logger *slog.Logger) *Renderer {
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"replace": func(from, to, input string) string {
			return strings.ReplaceAll(input, from, to)
		},
		"now": time.Now,
	}

	tc := getGlobalCache(templateDir)

	tc.mu.RLock()
	cacheValid := len(tc.templates) > 0 && !tc.hasTemplatesChanged()
	if cacheValid {
		r := &Renderer{
			Layout:      tc.templates["layout"],
			Index:       tc.templates["index"],
			Graph:       tc.templates["graph"],
			NotFound:    tc.templates["404"],
			Compress:    compress,
			DestFs:      destFs,
			RenderedSet: make(map[string]bool),
			logger:      logger,
		}
		tc.mu.RUnlock()
		return r
	}
	tc.mu.RUnlock()

	layoutPath := filepath.Join(templateDir, "layout.html")
	layoutContent, err := os.ReadFile(layoutPath)
	if err != nil {
		logger.Error("Failed to read layout template", "path", layoutPath, "error", err)
		os.Exit(1)
	}
	tmpl, err := template.New("layout.html").Funcs(funcMap).Parse(string(layoutContent))
	if err != nil {
		logger.Error("Failed to parse layout template", "path", layoutPath, "error", err)
		if strings.Contains(err.Error(), "template") && strings.Contains(err.Error(), "not defined") {
			logger.Error("Possible template cycle detected - check for circular {{ template }} references")
		}
		os.Exit(1)
	}
	layoutInfo, _ := os.Stat(layoutPath)
	if layoutInfo != nil {
		tc.setTemplate("layout", tmpl, layoutInfo.ModTime(), layoutContent)
	}

	indexPath := filepath.Join(templateDir, "index.html")
	var indexTmpl *template.Template
	indexContent, err := os.ReadFile(indexPath)
	if err != nil {
		logger.Warn("Index template not found, falling back to layout", "dir", templateDir, "error", err)
	} else {
		indexTmpl, err = template.New("index.html").Funcs(funcMap).Parse(string(indexContent))
		if err != nil {
			logger.Warn("Failed to parse index template", "dir", templateDir, "error", err)
		} else {
			indexInfo, _ := os.Stat(indexPath)
			if indexInfo != nil {
				tc.setTemplate("index", indexTmpl, indexInfo.ModTime(), indexContent)
			}
		}
	}

	graphPath := filepath.Join(templateDir, "graph.html")
	var graphTmpl *template.Template
	graphContent, err := os.ReadFile(graphPath)
	if err != nil {
		logger.Warn("Graph template not found, skipping graph page", "dir", templateDir, "error", err)
	} else {
		graphTmpl, err = template.ParseFiles(graphPath)
		if err != nil {
			logger.Warn("Failed to parse graph template", "dir", templateDir, "error", err)
		} else {
			graphInfo, _ := os.Stat(graphPath)
			if graphInfo != nil {
				tc.setTemplate("graph", graphTmpl, graphInfo.ModTime(), graphContent)
			}
		}
	}

	notFoundPath := filepath.Join(templateDir, "404.html")
	var notFoundTmpl *template.Template
	notFoundContent, err := os.ReadFile(notFoundPath)
	if err != nil {
		logger.Warn("404 template not found, falling back to layout", "dir", templateDir, "error", err)
	} else {
		notFoundTmpl, err = template.New("404.html").Funcs(funcMap).Parse(string(notFoundContent))
		if err != nil {
			logger.Warn("Failed to parse 404 template", "dir", templateDir, "error", err)
		} else {
			notFoundInfo, _ := os.Stat(notFoundPath)
			if notFoundInfo != nil {
				tc.setTemplate("404", notFoundTmpl, notFoundInfo.ModTime(), notFoundContent)
			}
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
		logger:      logger,
	}
}

func (r *Renderer) RegisterFile(path string) {
	r.RenderedMu.Lock()
	defer r.RenderedMu.Unlock()
	r.RenderedSet[filepath.ToSlash(path)] = true
}

func (r *Renderer) GetRenderedFiles() map[string]bool {
	r.RenderedMu.RLock()
	defer r.RenderedMu.RUnlock()
	copy := make(map[string]bool, len(r.RenderedSet))
	for k, v := range r.RenderedSet {
		copy[k] = v
	}
	return copy
}

func (r *Renderer) ClearRenderedFiles() {
	r.RenderedMu.Lock()
	defer r.RenderedMu.Unlock()
	r.RenderedSet = make(map[string]bool)
}

func (r *Renderer) SetAssets(assets map[string]string) {
	r.AssetsMu.Lock()
	defer r.AssetsMu.Unlock()
	r.Assets = assets
}

func (r *Renderer) GetAssets() map[string]string {
	r.AssetsMu.RLock()
	defer r.AssetsMu.RUnlock()
	copy := make(map[string]string, len(r.Assets))
	for k, v := range r.Assets {
		copy[k] = v
	}
	return copy
}
