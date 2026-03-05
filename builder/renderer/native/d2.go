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
func (r *Renderer) RenderD2(ctx context.Context, code string, themeID int64) (string, error) {
	r.ensureInitialized()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", fmt.Errorf("renderer is closed")
	}
	r.wg.Add(1)
	r.mu.Unlock()

	defer r.wg.Done()

	// Acquire worker with context awareness
	var instance *instance
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case instance = <-r.pool:
	}
	defer func() {
		r.mu.Lock()
		isClosed := r.closed
		r.mu.Unlock()
		if !isClosed {
			r.pool <- instance
		}
	}()

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
