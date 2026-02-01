// Package headless provides a chromedp-based renderer for Mermaid diagrams and LaTeX math.
// It uses a mutex to ensure only one diagram is rendered at a time, preventing RAM spikes.
package headless

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Orchestrator manages a headless Chrome instance for rendering diagrams
type Orchestrator struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	baseURL string // URL for static assets (e.g., "http://localhost:31415")
}

// New creates a new Orchestrator (Chrome is started lazily on first render)
func New() *Orchestrator {
	return &Orchestrator{}
}

// SetBaseURL sets the base URL for static assets (enables offline rendering)
func (o *Orchestrator) SetBaseURL(baseURL string) {
	o.baseURL = baseURL
}

// ensureStarted lazily initializes Chrome on first use
func (o *Orchestrator) ensureStarted() error {
	if o.started {
		return nil
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("allow-insecure-localhost", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
	)

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Warm up the browser
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return fmt.Errorf("failed to start Chrome: %w", err)
	}

	o.ctx = ctx
	o.cancel = cancel
	o.started = true
	log.Println("🌐 Headless Chrome started for diagram rendering")
	return nil
}

// Stop closes the Chrome instance
func (o *Orchestrator) Stop() {
	if o.started && o.cancel != nil {
		o.cancel()
		o.started = false
		log.Println("🌐 Headless Chrome stopped")
	}
}

// HashContent generates an MD5 hash for cache keys
func HashContent(contentType, content string) string {
	h := md5.New()
	h.Write([]byte(contentType + ":" + content))
	return hex.EncodeToString(h.Sum(nil))
}

// OptimizeSVG removes unnecessary attributes and minifies Mermaid SVG output
// Note: Colors are now preserved - Mermaid handles theming based on theme parameter
func OptimizeSVG(svg string) string {
	// Remove unnecessary id attributes (but preserve colors)
	svg = regexp.MustCompile(`\s+id="[^"]*"`).ReplaceAllString(svg, "")

	// Remove empty style attributes
	svg = regexp.MustCompile(`\s+style=""`).ReplaceAllString(svg, "")

	// Remove data-id attributes
	svg = regexp.MustCompile(`\s+data-id="[^"]*"`).ReplaceAllString(svg, "")

	// Remove data-node attributes
	svg = regexp.MustCompile(`\s+data-node="[^"]*"`).ReplaceAllString(svg, "")

	// Collapse multiple spaces
	svg = regexp.MustCompile(`\s+`).ReplaceAllString(svg, " ")

	// Remove spaces between tags
	svg = regexp.MustCompile(`>\s+<`).ReplaceAllString(svg, "><")

	return svg
}

// RenderMermaid renders a Mermaid diagram to SVG with the specified theme
// Theme should be "dark" or "light" - Mermaid will use its native theming
func (o *Orchestrator) RenderMermaid(code string, theme string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := o.ensureStarted(); err != nil {
		return "", err
	}

	// Default to dark theme if not specified
	if theme == "" {
		theme = "dark"
	}

	// Create a new tab for this render
	tabCtx, cancel := chromedp.NewContext(o.ctx)
	defer cancel()

	// Set a longer timeout for CDN loading
	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	// Determine asset URL (offline or CDN)
	mermaidURL := "https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js"
	if o.baseURL != "" {
		mermaidURL = o.baseURL + "/js/mermaid.min.js"
	}

	// Set viewport wider than site layout to give Mermaid room for proper text measurement
	// Using 1920px provides more space for accurate text measurement and better diagram sizing
	viewportWidth := int64(1920)
	viewportHeight := int64(1080)
	deviceScaleFactor := float64(1)
	mobile := false

	// Use Mermaid's API directly to render and capture SVG
	var svg string
	err := chromedp.Run(tabCtx,
		// Navigate to about:blank first
		chromedp.Navigate("about:blank"),
		// Set viewport to match site layout
		emulation.SetDeviceMetricsOverride(viewportWidth, viewportHeight, deviceScaleFactor, mobile),
		// Inject Mermaid script and wait for it to load
		chromedp.Evaluate(fmt.Sprintf(`
			new Promise((resolve, reject) => {
				// Inject Font CSS
				const style = document.createElement('style');
				style.textContent = '@import url("https://fonts.googleapis.com/css2?family=Inter:wght@400;600&display=swap"); body { font-family: "Inter", sans-serif; } .mermaid { font-family: "Inter", sans-serif !important; }';
				document.head.appendChild(style);

				const script = document.createElement('script');
				script.src = '%s';
				script.onload = () => resolve('loaded');
				script.onerror = () => reject('failed to load');
				document.head.appendChild(script);
			})
		`, mermaidURL), nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
		// Now render the diagram with the specified theme
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				try {
					mermaid.initialize({ 
						startOnLoad: false, 
						theme: '%s',
						fontFamily: '"Inter", sans-serif',
						flowchart: {
							useMaxWidth: true,
							diagramPadding: 20,
							nodeSpacing: 60,
							rankSpacing: 60,
							padding: 20,
							htmlLabels: true,
							wrap: false
						}
					});
					// Small delay to ensure font loads
					await new Promise(r => setTimeout(r, 100));
					const { svg } = await mermaid.render('diagram', %q);
					return svg;
				} catch(e) {
					return 'ERROR:' + e.message;
				}
			})()
		`, theme, code), &svg, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)

	if err != nil {
		return "", fmt.Errorf("mermaid render failed: %w", err)
	}

	if strings.HasPrefix(svg, "ERROR:") {
		return "", fmt.Errorf("mermaid parse error: %s", strings.TrimPrefix(svg, "ERROR:"))
	}

	// Optimize the SVG to reduce size
	optimizedSVG := OptimizeSVG(svg)

	return optimizedSVG, nil
}

// RenderMath renders a single LaTeX expression to HTML using KaTeX
func (o *Orchestrator) RenderMath(latex string, displayMode bool) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := o.ensureStarted(); err != nil {
		return "", err
	}

	// Create a new tab for this render
	tabCtx, cancel := chromedp.NewContext(o.ctx)
	defer cancel()

	// Set a timeout
	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 30*time.Second)
	defer timeoutCancel()

	displayModeJS := "false"
	if displayMode {
		displayModeJS = "true"
	}

	// Determine asset URLs (offline or CDN)
	katexCSS := "https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.css"
	katexJS := "https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.js"
	if o.baseURL != "" {
		katexCSS = o.baseURL + "/css/katex.min.css"
		katexJS = o.baseURL + "/js/katex.min.js"
	}

	var result string
	// Use data URL with proper DOCTYPE to avoid quirks mode
	htmlPage := `data:text/html,<!DOCTYPE html><html><head></head><body></body></html>`
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(htmlPage),
		// Load KaTeX via script injection with Promise
		chromedp.Evaluate(fmt.Sprintf(`
			new Promise((resolve, reject) => {
				const link = document.createElement('link');
				link.rel = 'stylesheet';
				link.href = '%s';
				document.head.appendChild(link);
				
				const script = document.createElement('script');
				script.src = '%s';
				script.onload = () => resolve('loaded');
				script.onerror = (e) => reject('Failed to load script from %s: ' + (e?.message || 'unknown error'));
				document.head.appendChild(script);
			})
		`, katexCSS, katexJS, katexJS), nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
		// Render the math expression
		chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				try {
					const container = document.createElement('div');
					katex.render(%q, container, {
						displayMode: %s,
						throwOnError: false,
						output: 'html'
					});
					return container.innerHTML;
				} catch(e) {
					return 'ERROR:' + e.message;
				}
			})()
		`, latex, displayModeJS), &result),
	)

	if err != nil {
		return "", fmt.Errorf("katex render failed: %w", err)
	}

	if strings.HasPrefix(result, "ERROR:") {
		return "", fmt.Errorf("katex parse error: %s", strings.TrimPrefix(result, "ERROR:"))
	}

	return result, nil
}

// MathExpression represents a LaTeX expression with its metadata
type MathExpression struct {
	LaTeX       string
	DisplayMode bool
	Hash        string
}

// checkCache filters out expressions that are already cached
func checkCache(expressions []MathExpression, cache map[string]string) []MathExpression {
	var toRender []MathExpression
	for _, expr := range expressions {
		if _, exists := cache[expr.Hash]; !exists {
			toRender = append(toRender, expr)
		}
	}
	return toRender
}

// RenderAllMath renders multiple LaTeX expressions in one Chrome session for efficiency
func (o *Orchestrator) RenderAllMath(expressions []MathExpression, cache map[string]string) (map[string]string, error) {
	if len(expressions) == 0 {
		return nil, nil
	}

	// Filter out cached expressions
	toRender := checkCache(expressions, cache)
	cachedCount := len(expressions) - len(toRender)

	if cachedCount > 0 {
		log.Printf("   📦 Using %d cached LaTeX expressions", cachedCount)
	}

	if len(toRender) == 0 {
		return nil, nil // All cached, no need to start Chrome
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if err := o.ensureStarted(); err != nil {
		return nil, err
	}

	log.Printf("   🔢 Rendering %d LaTeX expressions", len(toRender))

	// Determine asset URLs (offline or CDN)
	katexCSS := "https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.css"
	katexJS := "https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.js"
	if o.baseURL != "" {
		katexCSS = o.baseURL + "/css/katex.min.css"
		katexJS = o.baseURL + "/js/katex.min.js"
	}

	// Create a new tab for batch rendering
	tabCtx, cancel := chromedp.NewContext(o.ctx)
	defer cancel()

	// Set a longer timeout for batch
	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	// Load KaTeX first - use data URL with proper DOCTYPE to avoid quirks mode
	htmlPage := `data:text/html,<!DOCTYPE html><html><head></head><body></body></html>`
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(htmlPage),
		chromedp.Evaluate(fmt.Sprintf(`
			new Promise((resolve, reject) => {
				const link = document.createElement('link');
				link.rel = 'stylesheet';
				link.href = '%s';
				document.head.appendChild(link);
				
				const script = document.createElement('script');
				script.src = '%s';
				script.onload = () => resolve('loaded');
				script.onerror = (e) => reject('Failed to load script from %s: ' + (e?.message || 'unknown error'));
				document.head.appendChild(script);
			})
		`, katexCSS, katexJS, katexJS), nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to load KaTeX: %w", err)
	}

	// Render each expression
	results := make(map[string]string)
	for _, expr := range toRender {
		displayModeJS := "false"
		if expr.DisplayMode {
			displayModeJS = "true"
		}

		var result string
		err := chromedp.Run(tabCtx,
			chromedp.Evaluate(fmt.Sprintf(`
				(function() {
					try {
						const container = document.createElement('div');
						katex.render(%q, container, {
							displayMode: %s,
							throwOnError: false,
							output: 'html'
						});
						return container.innerHTML;
					} catch(e) {
						return 'ERROR:' + e.message;
					}
				})()
			`, expr.LaTeX, displayModeJS), &result),
		)

		if err != nil {
			log.Printf("   ⚠️  LaTeX render failed for %s: %v", expr.Hash[:8], err)
			continue
		}

		if strings.HasPrefix(result, "ERROR:") {
			log.Printf("   ⚠️  LaTeX parse error for %s: %s", expr.Hash[:8], strings.TrimPrefix(result, "ERROR:"))
			continue
		}

		results[expr.Hash] = result
	}

	log.Printf("   ✅ Rendered %d/%d LaTeX expressions", len(results), len(toRender))
	return results, nil
}
