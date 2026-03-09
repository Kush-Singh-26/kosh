// Package native provides a native Go renderer for D2 diagrams and LaTeX math.
package native

import (
	_ "embed"
	"encoding/hex"
	"log/slog"
	"runtime"
	"sync"

	"github.com/fastschema/qjs"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/singleflight"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

//go:generate go run ../../../scripts/compile-katex/main.go

//go:embed katex.min.js
var katexJS string

//go:embed katex.bytecode
var katexBytecode []byte

// instance represents a single isolated renderer worker
type instance struct {
	ruler         *textmeasure.Ruler
	rt            *qjs.Runtime
	ctx           *qjs.Context
	katex         *qjs.Value
	renderFn      *qjs.Value
	renderBatchFn *qjs.Value
	opts          *qjs.Value
	initOnce      sync.Once
}

// Renderer manages a pool of native rendering instances for concurrency
type Renderer struct {
	pool           chan *instance
	numWorkers     int
	initOnce       sync.Once
	katexBytecode  []byte
	wg             sync.WaitGroup
	mu             sync.Mutex
	closed         bool
	mathGroup      singleflight.Group
	D2Singleflight singleflight.Group // Shared group to deduplicate D2 diagram rendering across posts
}

type RendererOption func(*Renderer)

func WithWorkers(n int) RendererOption {
	return func(r *Renderer) {
		if n > 0 {
			r.numWorkers = n
		}
	}
}

// New creates a new Renderer - workers are lazy-initialized
func New(opts ...RendererOption) *Renderer {
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	r := &Renderer{
		pool:       make(chan *instance, numWorkers),
		numWorkers: numWorkers,
	}

	for _, opt := range opts {
		opt(r)
	}

	// Re-create pool with correct size if workers were changed
	if r.numWorkers != numWorkers {
		r.pool = make(chan *instance, r.numWorkers)
	}

	return r
}

// ensureInitialized lazily creates worker instances on first use
func (r *Renderer) ensureInitialized() {
	r.initOnce.Do(func() {
		slog.Info("Initializing QuickJS Renderer Pool", "workers", r.numWorkers)

		// Use pre-compiled bytecode if available, otherwise compile at runtime
		if len(katexBytecode) > 0 {
			slog.Info("Using pre-compiled KaTeX bytecode", "size", len(katexBytecode))
			r.katexBytecode = katexBytecode
		} else {
			slog.Info("Pre-compiling KaTeX to bytecode (fallback)")
			rt, err := qjs.New()
			if err == nil {
				bc, err := rt.Compile("katex.min.js", qjs.Code(katexJS))
				if err == nil {
					r.katexBytecode = bc
				} else {
					slog.Warn("Failed to compile KaTeX to bytecode", "error", err)
				}
				rt.Close()
			} else {
				slog.Warn("Failed to create temporary runtime for compilation", "error", err)
			}
		}

		// Start workers in background without blocking
		r.wg.Add(r.numWorkers)
		for i := 0; i < r.numWorkers; i++ {
			go func(id int) {
				defer r.wg.Done()
				instance := newinstance()
				if instance != nil {
					instance.ensureInitialized(r.katexBytecode)

					r.mu.Lock()
					isClosed := r.closed
					r.mu.Unlock()

					if !isClosed {
						r.pool <- instance
					}
				} else {
					slog.Warn("Failed to initialize worker", "id", id)
				}
			}(i)
		}
	})
}

// EnsureInitialized triggers lazy worker initialization eagerly.
func (r *Renderer) EnsureInitialized() {
	r.ensureInitialized()
}

func newinstance() *instance {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		slog.Warn("Failed to initialize text ruler", "error", err)
	}

	return &instance{
		ruler: ruler,
	}
}

// ensureInitialized performs lazy initialization of the JS engine
func (i *instance) ensureInitialized(bytecode []byte) {
	i.initOnce.Do(func() {
		rt, err := qjs.New()
		if err != nil {
			slog.Warn("Failed to create QJS runtime", "error", err)
			return
		}
		ctx := rt.Context()

		// Provide minimal browser environment for KaTeX
		_, err = ctx.Eval("init.js", qjs.Code(`
			var console = {
				log: function() {},
				warn: function() {},
				error: function() {}
			};
			var document = {
				createElement: function() {
					return {
						setAttribute: function() {}
					};
				}
			};
			var renderBatch = function(expressions) {
				return expressions.map(function(e) {
					try {
						return katex.renderToString(e.latex, {
							displayMode: e.displayMode,
							throwOnError: false,
							output: 'html'
						});
					} catch (err) {
						return "error: " + err.message;
					}
				});
			};
		`))
		if err != nil {
			slog.Warn("Failed to initialize JS environment", "error", err)
		}

		// Load KaTeX (Use bytecode if available, fallback to eval)
		if len(bytecode) > 0 {
			_, err = ctx.Eval("katex.min.js", qjs.Bytecode(bytecode))
		} else {
			_, err = ctx.Eval("katex.min.js", qjs.Code(katexJS))
		}

		if err != nil {
			slog.Warn("Failed to load KaTeX", "error", err)
			return
		}

		global := ctx.Global()
		defer global.Free()

		katex := global.GetPropertyStr("katex")
		if katex == nil || !katex.IsObject() {
			slog.Warn("KaTeX not found in global scope")
			return
		}

		renderToString := katex.GetPropertyStr("renderToString")
		if renderToString == nil || !renderToString.IsFunction() {
			slog.Warn("katex.renderToString is not a function")
			return
		}

		renderBatch := global.GetPropertyStr("renderBatch")
		if renderBatch == nil || !renderBatch.IsFunction() {
			slog.Warn("renderBatch is not a function")
		}

		i.rt = rt
		i.ctx = ctx
		i.katex = katex
		i.renderFn = renderToString
		i.renderBatchFn = renderBatch

		// Pre-create options object
		opts := ctx.NewObject()
		opts.SetPropertyStr("throwOnError", ctx.NewBool(false))
		opts.SetPropertyStr("output", ctx.NewString("html"))
		i.opts = opts
	})
}

// Close shuts down the renderer and cleans up worker resources.
func (r *Renderer) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	// Wait for all workers to be initialized AND all active tasks to complete
	r.wg.Wait()

	// Drain any workers that were back in the pool
	for i := 0; i < len(r.pool); i++ {
		instance := <-r.pool
		if instance.renderFn != nil {
			instance.renderFn.Free()
		}
		if instance.opts != nil {
			instance.opts.Free()
		}
		if instance.katex != nil {
			instance.katex.Free()
		}
		// Context is tied to runtime in this library
		if instance.rt != nil {
			instance.rt.Close()
		}
	}

	close(r.pool)
	return nil
}

// HashContent generates a XXH3 hash for cache keys
func HashContent(contentType, content string) string {
	h := xxh3.New()
	_, _ = h.WriteString(contentType + ":" + content)
	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:])[:16]
}

// GetD2Singleflight returns the shared singleflight group for D2 rendering
func (r *Renderer) GetD2Singleflight() *singleflight.Group {
	return &r.D2Singleflight
}
