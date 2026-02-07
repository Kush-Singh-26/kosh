package native

import (
	"context"
	"fmt"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/util-go/go2"
)

// RenderD2 renders a D2 diagram to SVG with the specified theme ID.
func (r *Renderer) RenderD2(code string, themeID int64) (string, error) {
	// Acquire worker
	instance := <-r.pool
	defer func() { r.pool <- instance }() // Release worker

	// Configure layout
	layout := func(ctx context.Context, g *d2graph.Graph) error {
		return d2dagrelayout.Layout(ctx, g, nil)
	}

	compileOpts := &d2lib.CompileOptions{
		Layout: nil,
		Ruler:  instance.ruler,
	}

	compileOpts.LayoutResolver = func(engine string) (d2graph.LayoutGraph, error) {
		return layout, nil
	}

	renderOpts := &d2svg.RenderOpts{
		ThemeID: &themeID,
		Pad:     go2.Pointer(int64(0)),
	}

	ctx := d2log.WithDefault(context.Background())

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
