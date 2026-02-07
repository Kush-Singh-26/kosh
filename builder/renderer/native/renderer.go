// Package native provides a native Go renderer for D2 diagrams and LaTeX math.
package native

import (
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"log"
	"runtime"
	"sync"

	"github.com/dop251/goja"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

//go:embed katex.min.js
var katexJS string

// Instance represents a single isolated renderer worker
type Instance struct {
	ruler    *textmeasure.Ruler
	vm       *goja.Runtime
	katex    goja.Value
	renderFn goja.Callable
	initOnce sync.Once
}

// Renderer manages a pool of native rendering instances for concurrency
type Renderer struct {
	pool chan *Instance
}

// New creates a new Renderer with a pool of workers sized to CPU count
func New() *Renderer {
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	pool := make(chan *Instance, numWorkers)

	log.Printf("⚙️  Initializing Renderer Pool with %d workers...", numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			instance := newInstance()
			if instance != nil {
				pool <- instance
			} else {
				log.Printf("⚠️ Failed to initialize worker %d", id)
			}
		}(i)
	}
	wg.Wait()

	return &Renderer{
		pool: pool,
	}
}

func newInstance() *Instance {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		log.Printf("⚠️ Failed to initialize text ruler: %v", err)
	}

	return &Instance{
		ruler: ruler,
	}
}

// ensureInitialized performs lazy initialization of the JS engine
func (i *Instance) ensureInitialized() {
	i.initOnce.Do(func() {
		// Initialize goja VM with KaTeX
		vm := goja.New()

		// Provide minimal console
		console := vm.NewObject()
		console.Set("log", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		console.Set("warn", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		console.Set("error", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
		vm.Set("console", console)

		// Document stub
		document := vm.NewObject()
		document.Set("createElement", func(call goja.FunctionCall) goja.Value {
			elem := vm.NewObject()
			elem.Set("setAttribute", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
			return elem
		})
		vm.Set("document", document)

		// Load KaTeX
		_, err := vm.RunString(katexJS)
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

// HashContent generates an MD5 hash for cache keys
func HashContent(contentType, content string) string {
	h := md5.New()
	h.Write([]byte(contentType + ":" + content))
	return hex.EncodeToString(h.Sum(nil))
}
