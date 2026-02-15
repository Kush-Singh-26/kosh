// Package native provides a native Go renderer for D2 diagrams and LaTeX math.
package native

import (
	_ "embed"
	"encoding/hex"
	"log"
	"runtime"
	"sync"

	"github.com/dop251/goja"
	"github.com/zeebo/blake3"
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
}

// New creates a new Renderer - workers are lazy-initialized
func New() *Renderer {
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Channel buffer is sized to numWorkers to prevent deadlocks
	// Workers are returned to pool after use, so this is safe as long as
	// we never create more workers than the buffer size.
	return &Renderer{
		pool:       make(chan *instance, numWorkers),
		numWorkers: numWorkers,
	}
}

// ensureInitialized lazily creates worker instances on first use
func (r *Renderer) ensureInitialized() {
	r.initOnce.Do(func() {
		log.Printf("⚙️  Initializing Renderer Pool with %d workers...", r.numWorkers)

		// 1. Compile KaTeX once
		log.Printf("   📝 Compiling KaTeX script...")
		prog, err := goja.Compile("katex.min.js", katexJS, true)
		if err != nil {
			log.Printf("❌ Failed to compile KaTeX: %v", err)
			return
		}
		r.katexProg = prog

		// Start workers in background without blocking
		for i := 0; i < r.numWorkers; i++ {
			go func(id int) {
				instance := newinstance()
				if instance != nil {
					// Pass the program to the instance (we could store it in instance or pass it during ensureInitialized)
					instance.ensureInitialized(r.katexProg)
					r.pool <- instance
				} else {
					log.Printf("⚠️ Failed to initialize worker %d", id)
				}
			}(i)
		}
		// We DO NOT wait for workers to be ready.
		// The pool channel will block consumers until at least one worker is available.
		// This "streams" workers as they come online, improving start time.
	})
}

func newinstance() *instance {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		log.Printf("⚠️ Failed to initialize text ruler: %v", err)
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
			log.Printf("⚠️ Failed to load KaTeX: %v", err)
			return
		}

		katex := vm.Get("katex")
		if katex == nil || goja.IsUndefined(katex) {
			log.Printf("⚠️ KaTeX not found in VM")
			return
		}

		katexObj := katex.ToObject(vm)
		renderToString := katexObj.Get("renderToString")
		renderFn, ok := goja.AssertFunction(renderToString)
		if !ok {
			log.Printf("⚠️ katex.renderToString is not a function")
			return
		}

		i.vm = vm
		i.katex = katex
		i.renderFn = renderFn
	})
}

// HashContent generates a BLAKE3 hash for cache keys
func HashContent(contentType, content string) string {
	h := blake3.New()
	_, _ = h.WriteString(contentType + ":" + content)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
