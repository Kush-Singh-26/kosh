package renderer

import (
	"maps"
)

func (r *Renderer) RegisterFile(path string) {
	r.RenderedMu.Lock()
	r.RenderedSet[path] = true
	// Invalidate snapshot so GetRenderedFiles rebuilds it lazily
	r.renderedSnapshot.Store(nil)
	r.RenderedMu.Unlock()
}

func (r *Renderer) GetRenderedFiles() map[string]bool {
	// Fast path: valid snapshot already exists
	if s := r.renderedSnapshot.Load(); s != nil {
		return *s
	}
	// Slow path: build snapshot under lock, then cache it
	r.RenderedMu.Lock()
	snapshot := make(map[string]bool, len(r.RenderedSet))
	maps.Copy(snapshot, r.RenderedSet)
	r.renderedSnapshot.Store(&snapshot)
	r.RenderedMu.Unlock()
	return snapshot
}

func (r *Renderer) ClearRenderedFiles() {
	r.RenderedMu.Lock()
	r.RenderedSet = make(map[string]bool)
	r.renderedSnapshot.Store(nil)
	r.RenderedMu.Unlock()
}
