package native

import (
	"context"
	"fmt"

	"github.com/Kush-Singh-26/kosh/builder/utils"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

// RenderD2 renders a D2 diagram to SVG with the specified theme ID.
func (r *Renderer) RenderD2(ctx context.Context, code string, themeID int64) (string, error) {
	if err := r.withSchedulerAndClosedCheck(ctx, utils.TaskD2); err != nil {
		return "", err
	}
	defer r.wg.Done()

	ruler := r.rulerPool.Get().(*textmeasure.Ruler)
	if ruler == nil {
		// Fallback or error if ruler is nil
		return "", fmt.Errorf("failed to get text ruler from pool")
	}
	defer r.rulerPool.Put(ruler)

	// Configure layout
	layout := func(ctx context.Context, g *d2graph.Graph) error {
		return d2dagrelayout.Layout(ctx, g, nil)
	}

	compileOpts := &d2lib.CompileOptions{
		Layout: nil,
		Ruler:  ruler,
	}

	compileOpts.LayoutResolver = func(engine string) (d2graph.LayoutGraph, error) {
		return layout, nil
	}

	renderOpts := &d2svg.RenderOpts{
		ThemeID: &themeID,
		Pad:     go2.Pointer(int64(0)),
	}

	// Use provided context instead of Background
	// Wrap with D2 default logger to silence warnings
	ctx = d2log.WithDefault(ctx)
	diagram, _, err := d2lib.Compile(ctx, code, compileOpts, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 compile failed: %w", err)
	}

	out, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 render failed: %w", err)
	}

	return string(out), nil
}
