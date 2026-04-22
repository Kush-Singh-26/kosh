package renderer

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

type mockDiagramCache struct {
	items map[string]any
}

func (m *mockDiagramCache) Get(key string) (any, bool) {
	val, ok := m.items[key]
	return val, ok
}

func TestRenderer_RenderFragment_D2Hydration(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("themes/blog/templates", 0755)
	
	// Create required templates to avoid os.Exit(1) in ReloadTemplates
	_ = afero.WriteFile(fs, "themes/blog/templates/layout.html", []byte(`
{{define "layout"}}{{template "base.html" .}}{{end}}
{{define "mytest"}}{{ "<!--KOSH_D2:abc123-->" | safeHTML }}{{end}}
`), 0644)
	_ = afero.WriteFile(fs, "themes/blog/templates/index.html", []byte(`{{define "index"}}{{template "base.html" .}}{{end}}`), 0644)
	_ = afero.WriteFile(fs, "themes/blog/templates/home.html", []byte(`{{define "home"}}{{template "base.html" .}}{{end}}`), 0644)
	_ = afero.WriteFile(fs, "themes/blog/templates/404.html", []byte(`{{define "404"}}{{template "base.html" .}}{{end}}`), 0644)
	
	sink := testutil.NewMemSink()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	cache := &mockDiagramCache{
		items: map[string]any{
			"d2:abc123": models.SSRThemePair{
				Light: "<svg>light</svg>",
				Dark:  "<svg>dark</svg>",
			},
		},
	}

	r := NewWithFs(Options{
		SourceFs:    fs,
		TemplateDir: "themes/blog/templates",
		LayoutsDir:  "themes/blog/templates",
		Sink:        sink,
		Diagrams:    cache,
		DevMode:     true,
		Logger:      logger,
	})

	data := models.PageData{
		RelativePrefix: "",
		Permalink:      "/test",
		// SSRD2 is explicitly nil/empty to test self-hydration
	}

	// mytest is a block defined in layout.html
	fragment, err := r.RenderFragment("home", "mytest", data)
	if err != nil {
		t.Fatalf("RenderFragment failed: %v", err)
	}

	got := string(fragment)
	if !strings.Contains(got, "<svg>light</svg>") || !strings.Contains(got, "<svg>dark</svg>") {
		t.Errorf("Fragment was not hydrated: %s", got)
	}
}
