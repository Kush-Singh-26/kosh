// Package shortcodes provides templating and rendering for Hugo-style shortcodes.
package shortcodes

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/afero"
)

var (
	// Shortcode regex: {{< name attr="val" >}}...{{< /name >}}
	// Note: We avoid backreferences (\1) because RE2 doesn't support them.
	// We'll validate the closing tag name in the processing logic.
	shortcodeRegex = regexp.MustCompile(`(?s)\{\{<\s*(\w+)\s*(.*?)\s*>\}\}(?:(.*?)\{\{<\s*/\s*(\w+)\s*>\}\})?`)
	// Argument regex: attr="val" or attr=val
	argRegex = regexp.MustCompile(`(\w+)\s*=\s*("([^"]*)"|'([^']*)'|(\S+))`)
)

// Processor handles shortcode discovery and rendering.
type Processor struct {
	templates map[string]*template.Template
}

// New creates a new shortcode processor, loading templates from the given directory.
func New(fs afero.Fs, themeShortcodesDir string) (*Processor, error) {
	p := &Processor{
		templates: make(map[string]*template.Template),
	}

	// 1. Load built-in shortcodes
	if err := p.loadBuiltins(); err != nil {
		return nil, fmt.Errorf("failed to load built-in shortcodes: %w", err)
	}

	// 2. Load theme shortcodes (override built-ins)
	if themeShortcodesDir != "" {
		exists, _ := afero.DirExists(fs, themeShortcodesDir)
		if exists {
			err := afero.Walk(fs, themeShortcodesDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || filepath.Ext(path) != ".html" {
					return err
				}

				name := strings.TrimSuffix(filepath.Base(path), ".html")
				content, err := afero.ReadFile(fs, path)
				if err != nil {
					return err
				}

				tmpl, err := template.New(name).Funcs(p.funcMap()).Parse(string(content))
				if err != nil {
					return fmt.Errorf("failed to parse shortcode template %s: %w", name, err)
				}

				p.templates[name] = tmpl
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to load theme shortcodes: %w", err)
			}
		}
	}

	return p, nil
}

// Process replaces all shortcode tags in the provided markdown with rendered HTML.
func (p *Processor) Process(markdown []byte) ([]byte, error) {
	if !bytes.Contains(markdown, []byte("{{<")) {
		return markdown, nil
	}

	result := shortcodeRegex.ReplaceAllFunc(markdown, func(match []byte) []byte {
		groups := shortcodeRegex.FindSubmatch(match)
		if len(groups) < 4 {
			return match
		}

		name := string(groups[1])
		rawArgs := string(groups[2])
		inner := ""
		if len(groups) >= 5 && groups[4] != nil {
			closingName := string(groups[4])
			if closingName != name {
				// Mismatched tags, we probably caught a closing tag of a different shortcode.
				// For now, treat it as self-closing or return original match.
				// Simple fix: only use inner if names match.
				return match
			}
			inner = string(groups[3])
		}

		tmpl, ok := p.templates[name]
		if !ok {
			// If shortcode not found, return the original tag
			return match
		}

		args := p.parseArgs(rawArgs)
		processedInner, err := p.Process([]byte(inner))
		if err != nil {
			processedInner = []byte(inner)
		}

		data := map[string]any{
			"Args":  args,
			"Inner": template.HTML(processedInner),
			"Name":  name,
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return []byte(fmt.Sprintf("<!-- Error rendering shortcode %s: %v -->", name, err))
		}

		return buf.Bytes()
	})

	return result, nil
}

func (p *Processor) parseArgs(raw string) map[string]string {
	args := make(map[string]string)
	matches := argRegex.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		key := m[1]
		val := m[3] // Quoted with "
		if val == "" {
			val = m[4] // Quoted with '
		}
		if val == "" {
			val = m[5] // Unquoted
		}
		args[key] = val
	}
	return args
}

func (p *Processor) loadBuiltins() error {
	for name, content := range builtins {
		tmpl, err := template.New(name).Funcs(p.funcMap()).Parse(content)
		if err != nil {
			return err
		}
		p.templates[name] = tmpl
	}
	return nil
}

func (p *Processor) funcMap() template.FuncMap {
	return template.FuncMap{
		"default": func(defaultValue, value any) any {
			if value == nil {
				return defaultValue
			}
			if s, ok := value.(string); ok && s == "" {
				return defaultValue
			}
			return value
		},
	}
}

// builtins contains the default shortcode templates.
var builtins = map[string]string{
	"youtube": `<div class="shortcode-youtube">
    <iframe src="https://www.youtube.com/embed/{{ index .Args "id" }}" 
            frameborder="0" allowfullscreen loading="lazy"></iframe>
</div>`,
	"figure": `<figure class="shortcode-figure {{ index .Args "class" }}" {{ with index .Args "size" }}style="max-width: {{ . }};"{{ end }}>
    <img src="{{ index .Args "src" }}" alt="{{ index .Args "alt" | default (index .Args "caption") }}" loading="lazy">
    {{ with index .Args "caption" }}<figcaption>{{ . }}</figcaption>{{ end }}
</figure>`,
	"callout": `<div class="shortcode-callout callout-{{ index .Args "type" | default "note" }}">
    {{ with index .Args "title" }}<div class="callout-title">{{ . }}</div>{{ end }}
    <div class="callout-content">{{ .Inner }}</div>
</div>`,
	"details": `<details class="shortcode-details">
    <summary>{{ index .Args "summary" | default "Details" }}</summary>
    <div class="details-content">{{ .Inner }}</div>
</details>`,
}
