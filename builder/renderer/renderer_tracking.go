package renderer

// RegisterFile records a rendered file path.
func (r *Renderer) RegisterFile(path string) {
	r.renderedFiles.Store(path, struct{}{})
}

// GetRenderedFiles returns a snapshot of rendered file paths.
func (r *Renderer) GetRenderedFiles() map[string]bool {
	result := make(map[string]bool)
	r.renderedFiles.Range(func(key, _ any) bool {
		result[key.(string)] = true
		return true
	})
	return result
}

// ClearRenderedFiles clears the tracked rendered files.
func (r *Renderer) ClearRenderedFiles() {
	r.renderedFiles.Range(func(key, _ any) bool {
		r.renderedFiles.Delete(key)
		return true
	})
}
