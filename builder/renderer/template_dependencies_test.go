package renderer

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func writeDependencyTemplate(t *testing.T, fs afero.Fs, path, content string) {
	t.Helper()
	if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(100, 0)
	if err := fs.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func newDependencyTestRenderer(t *testing.T) (*Renderer, *templateCache) {
	t.Helper()
	fs := afero.NewMemMapFs()
	root := t.TempDir()
	themeDir := filepath.Join(root, "theme")
	layoutsDir := filepath.Join(root, "layouts")
	for path, content := range map[string]string{
		filepath.Join(themeDir, "layout.html"):                         `{{ define "content" }}Layout{{ end }}`,
		filepath.Join(themeDir, "home.html"):                           `{{ define "content" }}Home{{ end }}`,
		filepath.Join(themeDir, "partials", "nested", "shared.html"):   "Theme partial",
		filepath.Join(layoutsDir, "partials", "nested", "shared.html"): "Site partial",
	} {
		writeDependencyTemplate(t, fs, path, content)
	}
	r := NewWithFs(Options{
		SourceFs: fs, TemplateDir: themeDir, LayoutsDir: layoutsDir, Logger: slog.Default(),
	})
	return r, getGlobalCache(themeDir, layoutsDir, false)
}

func TestTemplateCache_UnchangedDependenciesReuseTemplates(t *testing.T) {
	r, tc := newDependencyTestRenderer(t)
	layout, graph, notFound := r.Layout, r.Graph, r.NotFound
	for range 2 {
		tc.lastCheckNs.Store(0)
		r.ReloadTemplates()
		if r.Layout != layout || r.Graph != graph || r.NotFound != notFound {
			t.Fatal("unchanged dependencies caused templates to be reloaded")
		}
	}
	if graph == nil || notFound == nil {
		t.Fatal("embedded graph and 404 templates must remain cached")
	}
}

func TestTemplateCache_DependencyChanges(t *testing.T) {
	cases := []struct {
		name, dir, file, action string
	}{
		{"theme partial modified", "theme", "partials/nested/shared.html", "modify"},
		{"site partial modified", "layouts", "partials/nested/shared.html", "modify"},
		{"partial added", "theme", "partials/new.html", "add"},
		{"theme partial deleted", "theme", "partials/nested/shared.html", "delete"},
		{"site partial deleted", "layouts", "partials/nested/shared.html", "delete"},
		{"slot modified", "theme", "home.html", "modify"},
		{"optional slot added", "theme", "index.html", "add"},
		{"optional slot deleted", "theme", "home.html", "delete"},
		{"site slot override added", "layouts", "home.html", "add"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r, tc := newDependencyTestRenderer(t)
			dir := r.templateDir
			if tt.dir == "layouts" {
				dir = r.layoutsDir
			}
			path := filepath.Join(dir, filepath.FromSlash(tt.file))
			mutateTemplateDependency(t, r.SourceFs, path, tt.action)
			changed, err := tc.checkTemplatesOnDisk(r.SourceFs)
			if err != nil || !changed {
				t.Fatalf("dependency change detected = %v, error = %v", changed, err)
			}
			tc.lastCheckNs.Store(0)
			r.ReloadTemplates()
			changed, err = tc.checkTemplatesOnDisk(r.SourceFs)
			if err != nil || changed {
				t.Fatalf("cache did not settle after reload: changed = %v, error = %v", changed, err)
			}
			if tt.action == "delete" && tt.file == "home.html" && r.Home != nil {
				t.Fatal("deleted optional slot remains cached")
			}
		})
	}
}

func mutateTemplateDependency(t *testing.T, fs afero.Fs, path, action string) {
	t.Helper()
	switch action {
	case "delete":
		if err := fs.Remove(path); err != nil {
			t.Fatal(err)
		}
	case "add":
		writeDependencyTemplate(t, fs, path, "Added template")
	case "modify":
		writeDependencyTemplate(t, fs, path, "Changed template")
		stamp := time.Unix(200, 0)
		if err := fs.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTemplateCache_SlotOverrideFallbackWithSameMtime(t *testing.T) {
	r, tc := newDependencyTestRenderer(t)
	path := filepath.Join(r.layoutsDir, "home.html")
	writeDependencyTemplate(t, r.SourceFs, path, `{{ define "content" }}Site home{{ end }}`)
	changed, err := tc.checkTemplatesOnDisk(r.SourceFs)
	if err != nil || !changed {
		t.Fatalf("same-mtime override not detected: changed = %v, error = %v", changed, err)
	}
	tc.lastCheckNs.Store(0)
	r.ReloadTemplates()
	if got := r.Home.Lookup("content").Tree.Root.String(); got != "Site home" {
		t.Fatalf("override content = %q, want Site home", got)
	}

	if err := r.SourceFs.Remove(path); err != nil {
		t.Fatal(err)
	}
	changed, err = tc.checkTemplatesOnDisk(r.SourceFs)
	if err != nil || !changed {
		t.Fatalf("override removal not detected: changed = %v, error = %v", changed, err)
	}
	tc.lastCheckNs.Store(0)
	r.ReloadTemplates()
	if got := r.Home.Lookup("content").Tree.Root.String(); got != "Home" {
		t.Fatalf("fallback content = %q, want Home", got)
	}
	changed, err = tc.checkTemplatesOnDisk(r.SourceFs)
	if err != nil || changed {
		t.Fatalf("fallback cache did not settle: changed = %v, error = %v", changed, err)
	}
}

func TestTemplateCache_EmbeddedGraphHasNoDiskDependency(t *testing.T) {
	r, tc := newDependencyTestRenderer(t)
	writeDependencyTemplate(t, r.SourceFs, filepath.Join(r.templateDir, "graph.html"), "Unused graph slot")
	changed, err := tc.checkTemplatesOnDisk(r.SourceFs)
	if err != nil || changed {
		t.Fatalf("unused graph.html invalidated embedded graph: changed = %v, error = %v", changed, err)
	}
}
