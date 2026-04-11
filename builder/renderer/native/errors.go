package native

import "errors"

var (
	errRendererClosed       = errors.New("renderer is closed")
	errRulerPoolUnavailable = errors.New("failed to get text ruler from pool")
	errKaTeXNotInitialized  = errors.New("katex not initialized in worker")
	errRenderBatchNotInit   = errors.New("renderbatch not initialized in worker")
)
