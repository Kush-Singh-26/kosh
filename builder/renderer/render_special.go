package renderer

import (
	"fmt"
	"io"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Executor is a minimal interface for templates that can be executed.
// This allows the unified helper to work with any template-like type.
type Executor interface {
	Execute(wr io.Writer, data interface{}) error
}

func (r *Renderer) RenderIndex(path string, data models.PageData) error {
	r.mu.RLock()
	index := r.Index
	r.mu.RUnlock()

	if index == nil {
		return fmt.Errorf("required template index.html not found for %s", path)
	}

	return r.executeTemplateAndWrite(path, index, data, "index")
}

func (r *Renderer) RenderGraph(path string, data models.PageData) error {
	r.mu.RLock()
	graph := r.Graph
	r.mu.RUnlock()

	if graph == nil {
		return fmt.Errorf("graph template not loaded for %s", path)
	}

	return r.executeTemplateAndWrite(path, graph, data, "graph")
}

func (r *Renderer) Render404(path string, data models.PageData) error {
	r.mu.RLock()
	notFound := r.NotFound
	r.mu.RUnlock()

	if notFound == nil {
		return fmt.Errorf("required template 404.html not found for %s", path)
	}

	return r.executeTemplateAndWrite(path, notFound, data, "404")
}
