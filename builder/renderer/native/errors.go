package native

import "errors"

var (
	errRendererClosed       = errors.New("renderer is closed")
	errRulerPoolUnavailable = errors.New("failed to get text ruler from pool")
	errKaTeXNotInitialized  = errors.New("KaTeX not initialized in worker")
	errRenderBatchNotInit   = errors.New("renderBatch not initialized in worker")
)
