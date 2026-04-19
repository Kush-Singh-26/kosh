// Package shortcodes provides Hugo-like shortcode processing for Kosh.
package shortcodes

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/afero"
)

var (
	openingRegex = regexp.MustCompile(`(?s)^\{\{<\s*(\w+)\s*(.*?)\s*>\}\}`)
	argRegex     = regexp.MustCompile(`(\w+)\s*=\s*("([^"]*)"|'([^']*)'|(\S+))`)
)

// Processor handles shortcode parsing and rendering.
type Processor struct {
	templates map[string]*template.Template
	renderer  func([]byte) ([]byte, error)
}

// New creates a new shortcode processor.
func New(fs afero.Fs, themeShortcodesDir string) (*Processor, error) {
	p := &Processor{
		templates: make(map[string]*template.Template),
	}
	if err := p.loadBuiltins(); err != nil {
		return nil, fmt.Errorf("failed to load built-in shortcodes: %w", err)
	}
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

// SetRenderer sets the markdown renderer function for inner content.
func (p *Processor) SetRenderer(renderer func([]byte) ([]byte, error)) {
	p.renderer = renderer
}

// Process parses and renders all shortcodes in the given markdown.
func (p *Processor) Process(markdown []byte) ([]byte, error) {
	if !bytes.Contains(markdown, []byte("{{<")) {
		return markdown, nil
	}

	var result bytes.Buffer
	remaining := markdown

	for len(remaining) > 0 {
		idx := bytes.Index(remaining, []byte("{{<"))
		if idx == -1 {
			result.Write(remaining)
			break
		}

		result.Write(remaining[:idx])
		remaining = remaining[idx:]

		openingMatch := openingRegex.FindSubmatchIndex(remaining)
		if openingMatch == nil || openingMatch[0] != 0 {
			result.Write(remaining[:3])
			remaining = remaining[3:]
			continue
		}

		name := string(remaining[openingMatch[2]:openingMatch[3]])
		rawArgs := string(remaining[openingMatch[4]:openingMatch[5]])
		openingTagLen := openingMatch[1]

		// Look for the matching closing tag, ensuring we skip nested content
		closingPattern := regexp.MustCompile(fmt.Sprintf(`(?s)\{\{<\s*/\s*%s\s*>\}\}`, regexp.QuoteMeta(name)))
		closingLoc := closingPattern.FindIndex(remaining)

		if closingLoc != nil && closingLoc[0] >= openingTagLen {
			p.handlePairedShortcode(&result, name, rawArgs, remaining, openingTagLen, closingLoc)
			remaining = remaining[closingLoc[1]:]
		} else {
			if p.isMismatch(remaining, openingTagLen) {
				result.Write(remaining[:3])
				remaining = remaining[3:]
				continue
			}
			p.handleSingleShortcode(&result, name, rawArgs, remaining, openingTagLen)
			remaining = remaining[openingTagLen:]
		}
	}
	return result.Bytes(), nil
}

func (p *Processor) handlePairedShortcode(result *bytes.Buffer, name, rawArgs string, remaining []byte, openingTagLen int, closingLoc []int) {
	inner := remaining[openingTagLen:closingLoc[0]]
	processedInner, _ := p.Process(inner)

	// If a renderer is available, process the inner content as markdown.
	var finalInner string
	if p.renderer != nil && (name == "callout" || name == "details") {
		renderedInner, err := p.renderer(processedInner)
		if err == nil {
			finalInner = string(renderedInner)
		} else {
			finalInner = string(processedInner)
		}
	} else {
		finalInner = string(processedInner)
	}

	rendered, err := p.renderShortcode(name, rawArgs, finalInner)
	if err != nil {
		result.Write(remaining[:openingTagLen])
	} else {
		result.Write(rendered)
	}
}

func (p *Processor) handleSingleShortcode(result *bytes.Buffer, name, rawArgs string, remaining []byte, openingTagLen int) {
	rendered, err := p.renderShortcode(name, rawArgs, "")
	if err != nil {
		result.Write(remaining[:openingTagLen])
	} else {
		result.Write(rendered)
	}
}

func (p *Processor) isMismatch(remaining []byte, openingTagLen int) bool {
	// If no matching closing tag is found, we only fall back to non-paired if there is no other
	// closing tag before the next opening tag, which would suggest a mismatch.
	anyTagRegex := regexp.MustCompile(`(?s)\{\{<\s*(/?)\s*\w+`)
	nextTagMatch := anyTagRegex.FindSubmatchIndex(remaining[openingTagLen:])
	if nextTagMatch != nil {
		isClosing := nextTagMatch[2] != nextTagMatch[3] && remaining[openingTagLen+nextTagMatch[2]] == '/'
		return isClosing
	}
	return false
}

func (p *Processor) renderShortcode(name, rawArgs, inner string) ([]byte, error) {
	tmpl, ok := p.templates[name]
	if !ok {
		return nil, fmt.Errorf("shortcode %s not found", name)
	}
	args := p.parseArgs(rawArgs)
	data := map[string]any{
		"Args":  args,
		"Inner": template.HTML(inner),
		"Name":  name,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *Processor) parseArgs(raw string) map[string]string {
	args := make(map[string]string)
	matches := argRegex.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		key := m[1]
		val := ""
		switch {
		case m[3] != "" || (m[2] != "" && m[2][0] == '"'):
			val = m[3]
		case m[4] != "" || (m[2] != "" && m[2][0] == '\''):
			val = m[4]
		default:
			val = m[5]
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
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"markdown": func(s string) template.HTML {
			if p.renderer == nil || s == "" {
				return template.HTML(s)
			}
			// Unescape HTML entities that were escaped by the Go template engine
			// before passing to the markdown renderer.
			// Example: "&amp;" -> "&" so that the renderer sees the original "&"
			// and renders it correctly once.
			unescaped := html.UnescapeString(s)
			res, err := p.renderer([]byte(unescaped))
			if err != nil {
				return template.HTML(s)
			}
			// Strip the outer <p> and </p> if Goldmark wrapped it,
			// to keep captions as inline fragments if possible.
			out := string(res)
			if strings.HasPrefix(out, "<p>") && strings.HasSuffix(strings.TrimSpace(out), "</p>") {
				out = strings.TrimPrefix(out, "<p>")
				out = strings.TrimSpace(out)
				out = strings.TrimSuffix(out, "</p>")
			}
			return template.HTML(out)
		},
	}
}

var builtins = map[string]string{
	"youtube": `<div class="shortcode-youtube"><iframe src="https://www.youtube.com/embed/{{ index .Args "id" }}" frameborder="0" allowfullscreen loading="lazy"></iframe></div>`,
	"figure": `<figure class="shortcode-figure {{ index .Args "class" }}" {{ with index .Args "size" }}style="max-width: {{ . }};"{{ end }}><img src="{{ index .Args "src" }}" alt="{{ index .Args "alt" | default (index .Args "caption") }}" {{ with index .Args "width" }}width="{{ . }}"{{ end }} {{ with index .Args "height" }}height="{{ . }}"{{ end }} loading="lazy">{{ with index .Args "caption" }}<figcaption>{{ . | markdown }}</figcaption>{{ end }}</figure>`,
	"callout": `<div class="shortcode-callout callout-{{ index .Args "type" | default "note" }}">{{ with index .Args "title" }}<div class="callout-title">{{ . | markdown }}</div>{{ end }}<div class="callout-content">{{ .Inner }}</div></div>`,
	"details": `<details class="shortcode-details"><summary>{{ index .Args "summary" | default "Details" | markdown }}</summary><div class="details-content">{{ .Inner }}</div></details>`,
}
