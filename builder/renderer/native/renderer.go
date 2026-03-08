// Package native provides a native Go renderer for D2 diagrams and LaTeX math.
package native

import (
	_ "embed"
	"encoding/hex"
	"log/slog"
	"runtime"
	"sync"

	"github.com/dop251/goja"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/singleflight"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

//go:embed katex.min.js
var katexJS string

// instance represents a single isolated renderer worker
type instance struct {
	ruler    *textmeasure.Ruler
	vm       *goja.Runtime
	katex    goja.Value
	renderFn goja.Callable
	initOnce sync.Once
}

// Renderer manages a pool of native rendering instances for concurrency
type Renderer struct {
	pool       chan *instance
	numWorkers int
	initOnce   sync.Once
	katexProg  *goja.Program // Pre-compiled program to share across workers
	wg         sync.WaitGroup
	mu         sync.Mutex
	closed     bool
	mathGroup  singleflight.Group
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
		slog.Info("Initializing Renderer Pool", "workers", r.numWorkers)

		// 1. Compile KaTeX once
		slog.Info("Compiling KaTeX script")
		prog, err := goja.Compile("katex.min.js", katexJS, true)
		if err != nil {
			slog.Error("Failed to compile KaTeX", "error", err)
			return
		}

		// Start workers in background without blocking
		// Pass program directly to workers to ensure happens-before guarantee
		r.wg.Add(r.numWorkers)
		for i := 0; i < r.numWorkers; i++ {
			go func(id int, p *goja.Program) {
				defer r.wg.Done()
				instance := newinstance()
				if instance != nil {
					instance.ensureInitialized(p)

					r.mu.Lock()
					isClosed := r.closed
					r.mu.Unlock()

					if !isClosed {
						r.pool <- instance
					}
				} else {
					slog.Warn("Failed to initialize worker", "id", id)
				}
			}(i, prog)
		}

		// Set katexProg after spawning workers (not needed by workers anymore since they have direct parameter)
		r.katexProg = prog

		// We DO NOT wait for workers to be ready.
		// The pool channel will block consumers until at least one worker is available.
		// This "streams" workers as they come online, improving start time.
	})
}

// EnsureInitialized triggers lazy worker initialization eagerly.
// Call this during setup to overlap KaTeX compilation with other init work.
// Safe to call concurrently or multiple times (guarded by sync.Once).
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
func (i *instance) ensureInitialized(prog *goja.Program) {
	i.initOnce.Do(func() {
		// Initialize goja VM with KaTeX
		vm := goja.New()

		// Provide minimal console
		console := vm.NewObject()
		_ = console.Set("log", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = console.Set("warn", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = console.Set("error", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = vm.Set("console", console)

		// Document stub
		document := vm.NewObject()
		_ = document.Set("createElement", func(call goja.FunctionCall) goja.Value {
			elem := vm.NewObject()
			_ = elem.Set("setAttribute", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
			return elem
		})
		_ = vm.Set("document", document)

		// Load KaTeX (Use pre-compiled program)
		_, err := vm.RunProgram(prog)
		if err != nil {
			slog.Warn("Failed to load KaTeX", "error", err)
			return
		}

		katex := vm.Get("katex")
		if katex == nil || goja.IsUndefined(katex) {
			slog.Warn("KaTeX not found in VM")
			return
		}

		katexObj := katex.ToObject(vm)
		renderToString := katexObj.Get("renderToString")
		renderFn, ok := goja.AssertFunction(renderToString)
		if !ok {
			slog.Warn("katex.renderToString is not a function")
			return
		}

		i.vm = vm
		i.katex = katex
		i.renderFn = renderFn
	})
}

// Close shuts down the renderer and cleans up worker resources.
// Safe to call multiple times.
func (r *Renderer) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	r.initOnce.Do(func() {
		// Ensure initialization doesn't happen during close
	})

	// Wait for all workers to be initialized AND all active tasks to complete
	r.wg.Wait()

	// Drain any workers that were back in the pool
	// They won't be reused since r.closed is true
	for i := 0; i < len(r.pool); i++ {
		<-r.pool
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
