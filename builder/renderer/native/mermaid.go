package native

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	mermaid "github.com/bvolpato/mermaid-go-renderer"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// RenderGlobalMermaidBatch renders all unique Mermaid diagrams across the entire site
// in parallel using the worker pool.
func (r *Renderer) RenderGlobalMermaidBatch(ctx context.Context, expressions []models.MermaidExpression) (map[string]models.SSRThemePair, error) {
	if len(expressions) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	uniqueExprs := make([]models.MermaidExpression, 0, len(expressions))
	seen := make(map[string]bool)
	for _, e := range expressions {
		if !seen[e.Hash] {
			seen[e.Hash] = true
			uniqueExprs = append(uniqueExprs, e)
		}
	}

	if len(uniqueExprs) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	slog.Info("Global Mermaid batch render", "total", len(uniqueExprs), "workers", r.numWorkers)
	timer := timeutil.StartPhase("Global Mermaid render")
	defer timer.Stop()

	return r.RenderAllMermaid(ctx, uniqueExprs, nil)
}

func (r *Renderer) spawnMermaidWorker(ctx context.Context, taskChan <-chan models.MermaidExpression, results map[string]models.SSRThemePair, mu *sync.Mutex, wg *sync.WaitGroup, globalErr *error) {
	wg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    slog.Default(),
		Operation: "mermaid render",
		Fn: func() error {
			for expr := range taskChan {
				lightSVG, err := r.RenderMermaid(ctx, expr.Code, false)
				if err != nil {
					mu.Lock()
					if *globalErr == nil {
						*globalErr = fmt.Errorf("mermaid render (light): %w", err)
					}
					mu.Unlock()
					slog.Warn("Mermaid light render failed", "hash", expr.Hash, "error", err)
					continue
				}

				darkSVG, err := r.RenderMermaid(ctx, expr.Code, true)
				if err != nil {
					mu.Lock()
					if *globalErr == nil {
						*globalErr = fmt.Errorf("mermaid render (dark): %w", err)
					}
					mu.Unlock()
					slog.Warn("Mermaid dark render failed", "hash", expr.Hash, "error", err)
					continue
				}

				mu.Lock()
				results[expr.Hash] = models.SSRThemePair{Light: lightSVG, Dark: darkSVG}
				mu.Unlock()
			}
			return nil
		},
		Cleanup: wg.Done,
	})
}

// RenderAllMermaid renders multiple Mermaid diagrams in parallel.
func (r *Renderer) RenderAllMermaid(ctx context.Context, expressions []models.MermaidExpression, cache map[string]models.SSRThemePair) (map[string]models.SSRThemePair, error) {
	if len(expressions) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	finalResults := make(map[string]models.SSRThemePair)
	var toRender []models.MermaidExpression
	for _, expr := range expressions {
		if val, ok := cache[expr.Hash]; ok {
			finalResults[expr.Hash] = val
		} else {
			toRender = append(toRender, expr)
		}
	}

	if len(toRender) == 0 {
		return finalResults, nil
	}

	numWorkers := min(r.numWorkers, len(toRender))
	results := make(map[string]models.SSRThemePair)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var globalErr error

	taskChan := make(chan models.MermaidExpression, len(toRender))
	for _, expr := range toRender {
		taskChan <- expr
	}
	close(taskChan)

	for i := 0; i < numWorkers; i++ {
		r.spawnMermaidWorker(ctx, taskChan, results, &mu, &wg, &globalErr)
	}

	wg.Wait()

	for hash, pair := range results {
		finalResults[hash] = pair
	}
	return finalResults, globalErr
}

var nodeLabelBracketRe = regexp.MustCompile(`\[([^\]]+)\]`)

// preprocessMermaidCode replaces '&' in bracketed node labels with a fullwidth ampersand ＆ (\uff06)
// so mermaid-go-renderer's lexer does not split node labels containing ampersands.
func preprocessMermaidCode(code string) string {
	return nodeLabelBracketRe.ReplaceAllStringFunc(code, func(match string) string {
		return strings.ReplaceAll(match, "&", "＆")
	})
}

// RenderMermaid renders a single Mermaid diagram to SVG.
// The isDark parameter selects the dark theme variant.
func (r *Renderer) RenderMermaid(ctx context.Context, code string, isDark bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if r.scheduler != nil {
		if err := r.scheduler.Acquire(ctx, scheduler.TaskMermaid); err != nil {
			return "", err
		}
		defer r.scheduler.Release(scheduler.TaskMermaid)
	}

	r.mu.Lock()
	if r.isClosed {
		r.mu.Unlock()
		return "", errRendererClosed
	}
	r.taskWg.Add(1)
	r.mu.Unlock()

	defer r.taskWg.Done()

	code = preprocessMermaidCode(code)

	opts := mermaid.DefaultRenderOptions()
	if isDark {
		opts.Theme = darkMermaidTheme()
	} else {
		opts.Theme = lightMermaidTheme()
	}

	svg, err := mermaid.RenderWithOptions(code, opts)
	if err != nil {
		return "", fmt.Errorf("mermaid render failed: %w", err)
	}

	return cleanMermaidSVG(svg), nil
}

// svgStyleAttrRe matches the inline style attribute on the root <svg> element.
var svgStyleAttrRe = regexp.MustCompile(`(<svg[^>]*?)style="[^"]*"`)

// svgViewBoxRe matches the viewBox attribute and captures minX, minY, width, height.
var svgViewBoxRe = regexp.MustCompile(`\bviewBox="([-0-9.eE]+)[,\s]+([-0-9.eE]+)[,\s]+([-0-9.eE]+)[,\s]+([-0-9.eE]+)"`)

// svgWidthHundredRe matches the width="100%" attribute on the root <svg> element.
var svgWidthHundredRe = regexp.MustCompile(`\bwidth="100%"`)

// svgHeightAttrRe matches an existing height attribute.
var svgHeightAttrRe = regexp.MustCompile(`\bheight="`)

// svgRootOpenRe matches the root <svg> opening tag.
var svgRootOpenRe = regexp.MustCompile(`(?is)(<svg\b[^>]*>)`)

// fixMermaidSVGSize replaces the library's responsive width="100%" with the
// diagram's natural pixel dimensions derived from the viewBox. This gives the
// SVG a real intrinsic size so CSS can render it at native scale (with
// horizontal scroll when wider than the article column) instead of stretching
// or squishing it to the container width.
func fixMermaidSVGSize(svg string) string {
	rootMatch := svgRootOpenRe.FindStringSubmatchIndex(svg)
	if rootMatch == nil {
		return svg
	}
	rootTag := svg[rootMatch[0]:rootMatch[1]]
	vm := svgViewBoxRe.FindStringSubmatch(rootTag)
	if len(vm) < 5 {
		return svg
	}
	w, errW := strconv.ParseFloat(vm[3], 64)
	h, errH := strconv.ParseFloat(vm[4], 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return svg
	}
	wStr := strconv.FormatFloat(w, 'f', -1, 64)
	hStr := strconv.FormatFloat(h, 'f', -1, 64)

	newTag := svgWidthHundredRe.ReplaceAllString(rootTag, `width="`+wStr+`"`)
	if !svgHeightAttrRe.MatchString(newTag) {
		closing := len(newTag) - 1
		newTag = newTag[:closing] + ` height="` + hStr + `"` + newTag[closing:]
	}
	return svg[:rootMatch[0]] + newTag + svg[rootMatch[1]:]
}

// mermaidPointEndMarkerRe matches the flowchart pointEnd arrowhead marker as
// emitted by mermaid-go-renderer (before SVG minification).
var mermaidPointEndMarkerRe = regexp.MustCompile(`<marker id="([^"]*-pointEnd)" class="marker flowchart-v2" viewBox="0 0 10 10" refX="5" refY="5" markerUnits="userSpaceOnUse" markerWidth="8" markerHeight="8" orient="auto"><path d="M 0 0 L 10 5 L 0 10 z"`)

// mermaidPointStartMarkerRe matches the mirrored flowchart pointStart marker.
var mermaidPointStartMarkerRe = regexp.MustCompile(`<marker id="([^"]*-pointStart)" class="marker flowchart-v2" viewBox="0 0 10 10" refX="4.5" refY="5" markerUnits="userSpaceOnUse" markerWidth="8" markerHeight="8" orient="auto"><path d="M 0 5 L 10 10 L 10 0 z"`)

// upgradeMermaidArrows enlarges the flowchart arrowhead markers to roughly 1.5x
// their original size so arrows read clearly at natural diagram scale. The tip
// overlap past the marker anchor point is preserved (5 viewBox units), so tips
// still land exactly on node borders.
func upgradeMermaidArrows(svg string) string {
	svg = mermaidPointEndMarkerRe.ReplaceAllString(svg, `<marker id="$1" class="marker flowchart-v2" viewBox="0 0 12 12" refX="7" refY="6" markerUnits="userSpaceOnUse" markerWidth="14" markerHeight="14" orient="auto"><path d="M 1 1 L 12 6 L 1 11 z"`)
	svg = mermaidPointStartMarkerRe.ReplaceAllString(svg, `<marker id="$1" class="marker flowchart-v2" viewBox="0 0 12 12" refX="6" refY="6" markerUnits="userSpaceOnUse" markerWidth="14" markerHeight="14" orient="auto"><path d="M 12 1 L 1 6 L 12 11 z"`)
	return svg
}

// cleanMermaidSVG post-processes the SVG output from mermaid-go-renderer to
// remove inline styles that conflict with CSS, restore ampersands, adjust font sizing,
// and inject the mermaid-svg class.
func cleanMermaidSVG(svg string) string {
	svg = svgStyleAttrRe.ReplaceAllString(svg, "$1")
	svg = fixMermaidSVGSize(svg)
	svg = strings.Replace(svg, `class="`, `class="mermaid-svg `, 1)

	// Restore fullwidth ampersand ＆ back to HTML entity &amp;
	svg = strings.ReplaceAll(svg, "＆", "&amp;")

	// Inject explicit resets into internal <style> tag so foreignObject elements inherit margin 0 and consistent typography
	svg = strings.Replace(svg, "</style>", "foreignObject, foreignObject div, foreignObject span, foreignObject p { margin: 0 !important; padding: 0 !important; line-height: 1.35 !important; overflow: visible !important; }</style>", 1)

	// Enlarge flowchart arrowheads after all other processing
	svg = upgradeMermaidArrows(svg)

	return svg
}

// lightMermaidTheme returns a light theme for Mermaid diagrams using the blog's
// sage/gold/blue palette (matches D2 lightOverrides).
func lightMermaidTheme() mermaid.Theme {
	return mermaid.Theme{
		Background:          "#F9F4EE", // n7 bg-body
		PrimaryColor:        "#425C4E", // b1 sage
		PrimaryBorderColor:  "#55695D", // b6 muted sage
		PrimaryTextColor:    "#100A03", // n1 text-main
		LineColor:           "#847C6C", // n5
		SecondaryColor:      "#59441F", // b2 aged gold
		TertiaryColor:       "#2E4D57", // b3 scholar blue
		EdgeLabelBackground: "#F9F4EE", // n7 bg-body
		ClusterBorder:       "#ABA28F", // n6
		TextColor:           "#100A03", // n1 text-main
		FontFamily:          "Inter, ui-sans-serif, system-ui, -apple-system, \"Segoe UI\", sans-serif",
		FontSize:            18,
	}
}

// darkMermaidTheme returns a dark theme for Mermaid diagrams using the blog's
// sage/gold/blue palette (matches D2 darkOverrides).
func darkMermaidTheme() mermaid.Theme {
	return mermaid.Theme{
		Background:          "#060A08", // n7 bg-body
		PrimaryColor:        "#7B9787", // b1 sage
		PrimaryBorderColor:  "#71867A", // b6 muted sage
		PrimaryTextColor:    "#D9E0DC", // n1 text-main
		LineColor:           "#646C65", // n4
		SecondaryColor:      "#B29B74", // b2 aged gold
		TertiaryColor:       "#93B4C0", // b3 scholar blue
		EdgeLabelBackground: "#060A08", // n7 bg-body
		ClusterBorder:       "#404841", // n5
		TextColor:           "#D9E0DC", // n1 text-main
		FontFamily:          "Inter, ui-sans-serif, system-ui, -apple-system, \"Segoe UI\", sans-serif",
		FontSize:            18,
	}
}
