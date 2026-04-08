package renderer

import (
	"fmt"
	"io"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Executor is a minimal interface for templates that can be executed.
// This allows the unified helper to work with any template-like type.
type Executor interface {
	Execute(wr io.Writer, data any) error
}

// RenderIndex renders the homepage using the index template.
func (r *Renderer) RenderIndex(path string, data models.PageData) error {
	r.mu.RLock()
	index := r.Index
	r.mu.RUnlock()

	if index == nil {
		return fmt.Errorf("required template index.html not found for %s", path)
	}

	return r.executeTemplateAndWrite(path, index, data, "index")
}

// RenderGraph renders the graph page using the graph template.
func (r *Renderer) RenderGraph(path string, data models.PageData) error {
	r.mu.RLock()
	graph := r.Graph
	layout := r.Layout
	r.mu.RUnlock()

	if graph == nil {
		r.logger.Warn("Graph template not found, skipping graph page")
		return nil
	}

	if layout == nil {
		r.logger.Warn("Layout template not loaded, skipping graph page")
		return fmt.Errorf("layout template not loaded for %s", path)
	}

	r.logger.Info("Rendering graph page", "path", path)
	if err := r.executeTemplateAndWrite(path, layout, data, "graph"); err != nil {
		return err
	}
	r.logger.Info("Graph page rendered successfully", "path", path)
	return nil
}

// Render404 renders the 404 page using the 404 template.
func (r *Renderer) Render404(path string, data models.PageData) error {
	r.mu.RLock()
	notFound := r.NotFound
	r.mu.RUnlock()

	if notFound == nil {
		return fmt.Errorf("required template 404.html not found for %s", path)
	}

	return r.executeTemplateAndWrite(path, notFound, data, "404")
}
