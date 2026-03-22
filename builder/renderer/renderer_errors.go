package renderer

import (
	"fmt"
)

type renderError struct {
	msg  string
	path string
	err  error
}

// recordError logs a render error and stores it for later retrieval
func (r *Renderer) recordError(msg string, path string, err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	r.renderErrors = append(r.renderErrors, renderError{msg: msg, path: path, err: err})
	r.logger.Error(msg, "path", path, "error", err)
}

// ConsumeErrors returns all accumulated render errors and clears the error list
func (r *Renderer) ConsumeErrors() []error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if len(r.renderErrors) == 0 {
		return nil
	}
	result := make([]error, len(r.renderErrors))
	for i, e := range r.renderErrors {
		result[i] = fmt.Errorf("%s (path: %s): %w", e.msg, e.path, e.err)
	}
	r.renderErrors = nil // Clear after retrieval
	return result
}
