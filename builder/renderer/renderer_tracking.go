package renderer

func (r *Renderer) RegisterFile(path string) {
	r.renderedFiles.Store(path, struct{}{})
}

func (r *Renderer) GetRenderedFiles() map[string]bool {
	result := make(map[string]bool)
	r.renderedFiles.Range(func(key, _ any) bool {
		result[key.(string)] = true
		return true
	})
	return result
}

func (r *Renderer) ClearRenderedFiles() {
	r.renderedFiles.Range(func(key, _ any) bool {
		r.renderedFiles.Delete(key)
		return true
	})
}
