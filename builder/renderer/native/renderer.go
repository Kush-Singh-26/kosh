// Package native provides a native Go renderer for D2 diagrams, Mermaid diagrams, and LaTeX math.
package native

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"runtime"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"

	"github.com/fastschema/qjs"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/singleflight"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const (
	minWorkers = 1

	bytecodeHeaderSize      = 20
	bytecodeMagic           = "KBC1"
	bytecodeMagicSize       = 4
	bytecodeSourceHashStart = 4
	bytecodeSourceHashEnd   = 12
	bytecodeSizeStart       = 12
	bytecodeSizeEnd         = 20
	bytecodePayloadOffset   = 20

	maxExecutionTimeMs = 2000
	hashPrefixLength   = 16
)

//go:generate go run ../../../scripts/compile-katex/main.go

//go:embed katex.min.js
var katexJS string

//go:embed katex.bytecode
var katexBytecode []byte

//go:embed pele.global.js
var peleJS string

// peleExportSuffix exposes the bundle's `var pele` binding on the global object.
const peleExportSuffix = "\n;globalThis.pele=pele;"

// Renderer manages a pool of native rendering instances for concurrency
type Renderer struct {
	ctx           context.Context
	pool          chan *instance
	numWorkers    int
	initReady     chan struct{}
	scheduler     scheduler.BuildScheduler
	mu            sync.Mutex
	isClosed      bool
	taskWg        sync.WaitGroup
	katexBytecode []byte
	peleBytecode  []byte
	rulerPool     sync.Pool

	D2Singleflight      singleflight.Group
	MermaidSingleflight singleflight.Group
	initOnce            sync.Once

	d2ThemeLight  int64
	d2ThemeDark   int64
	d2Scale       float64
	d2Pad         int64
	d2Sketch      bool
	d2StrokeScale float64

	d2LightOverrides *d2target.ThemeOverrides
	d2DarkOverrides  *d2target.ThemeOverrides
}

// RendererOption configures a Renderer.
type RendererOption func(*Renderer)

// WithContext sets the base context for background operations in the renderer.
func WithContext(ctx context.Context) RendererOption {
	return func(r *Renderer) {
		if ctx != nil {
			r.ctx = ctx
		}
	}
}

// WithWorkers sets the worker pool size for the renderer.
func WithWorkers(numWorkers int) RendererOption {
	return func(r *Renderer) {
		if numWorkers > 0 {
			r.numWorkers = numWorkers
		}
	}
}

// WithScheduler sets the build scheduler used by the renderer.
func WithScheduler(s scheduler.BuildScheduler) RendererOption {
	return func(r *Renderer) {
		r.scheduler = s
	}
}

// WithD2Config sets D2 diagram rendering options from the site config.
func WithD2Config(cfg models.D2Config) RendererOption {
	return func(r *Renderer) {
		r.d2ThemeLight = cfg.LightThemeID
		r.d2ThemeDark = cfg.DarkThemeID
		if cfg.Scale > 0 {
			r.d2Scale = cfg.Scale
		}
		r.d2Pad = cfg.Pad
		r.d2Sketch = cfg.Sketch
		// strokeWidth scales the rendered line/shape thickness. 0 (unset)
		// defaults to 1.0 (no change) to avoid regressing existing sites.
		if cfg.StrokeWidth > 0 {
			r.d2StrokeScale = cfg.StrokeWidth
		} else {
			r.d2StrokeScale = 1.0
		}
		r.d2LightOverrides = cfg.LightOverrides.ToD2Target()
		r.d2DarkOverrides = cfg.DarkOverrides.ToD2Target()
	}
}

// New creates a new Renderer - workers are lazy-initialized
func New(opts ...RendererOption) *Renderer {
	numWorkers := max(runtime.NumCPU(), minWorkers)

	r := &Renderer{
		ctx:  context.Background(),
		pool: make(chan *instance, numWorkers),
		rulerPool: sync.Pool{
			New: func() any {
				ruler, _ := textmeasure.NewRuler()
				return ruler
			},
		},
		numWorkers:   numWorkers,
		initReady:    make(chan struct{}),
		scheduler:    nil, // Must be set via WithScheduler option
		d2ThemeLight: 0,
		d2ThemeDark:  200,
		d2Pad:        0,
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

func (r *Renderer) validateBytecode(bytecode []byte, source string) ([]byte, bool) {
	if len(bytecode) < bytecodeHeaderSize {
		return nil, false
	}
	if string(bytecode[0:bytecodeMagicSize]) != bytecodeMagic {
		return nil, false
	}
	// Check hash of source JS
	sourceHash := binary.LittleEndian.Uint64(bytecode[bytecodeSourceHashStart:bytecodeSourceHashEnd])
	actualHash := xxh3.Hash([]byte(source))
	if sourceHash != actualHash {
		return nil, false
	}
	// Check size integrity
	expectedSize := binary.LittleEndian.Uint64(bytecode[bytecodeSizeStart:bytecodeSizeEnd])
	if uint64(len(bytecode)-bytecodeHeaderSize) != expectedSize {
		return nil, false
	}
	return bytecode[bytecodePayloadOffset:], true
}

// ensureInitialized lazily creates worker instances on first use
// compileJS compiles JavaScript source into QuickJS bytecode using a temporary
// runtime. Returns nil when compilation fails.
func compileJS(name, source string) []byte {
	rt, err := qjs.New(qjs.Option{MaxExecutionTime: maxExecutionTimeMs})
	if err != nil {
		slog.Warn("Failed to create temporary runtime for compilation", "name", name, "error", err)
		return nil
	}
	defer rt.Close()

	bytecode, err := rt.Compile(name, qjs.Code(source))
	if err != nil {
		slog.Warn("Failed to compile JS to bytecode", "name", name, "error", err)
		return nil
	}
	return bytecode
}

func (r *Renderer) ensureInitialized() {
	r.initOnce.Do(func() {
		slog.Info("Initializing QuickJS Renderer Pool", "workers", r.numWorkers)

		// Validate embedded KaTeX bytecode, or compile it from source.
		if bytecode, ok := r.validateBytecode(katexBytecode, katexJS); ok {
			slog.Info("Using validated KaTeX bytecode", "size", len(bytecode))
			r.katexBytecode = bytecode
		} else {
			if len(katexBytecode) > 0 {
				slog.Warn("KaTeX bytecode validation failed (stale or invalid), recompiling...")
			} else {
				slog.Info("No KaTeX bytecode found, compiling from source...")
			}
			r.katexBytecode = compileJS("katex.min.js", katexJS)
		}

		// Pele (Mermaid renderer) is compiled from embedded source on startup.
		r.peleBytecode = compileJS("pele.global.js", peleJS+peleExportSuffix)

		// Start workers in background without blocking
		var initWg sync.WaitGroup
		initWg.Add(r.numWorkers)
		for i := 0; i < r.numWorkers; i++ {
			async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
				Ctx:       r.ctx,
				Logger:    slog.Default(),
				Operation: "native renderer init worker",
				Fn: func() error {
					instance := &instance{}
					instance.ensureInitialized(r.katexBytecode, r.peleBytecode)

					r.mu.Lock()
					isClosed := r.isClosed
					r.mu.Unlock()

					if !isClosed {
						r.pool <- instance
					}
					return nil
				},
				Cleanup: initWg.Done,
			})
		}

		// Close initReady once all workers are started. This must not be gated on
		// r.ctx: Close() cancels the engine context before shutting the renderer
		// down, and an unclosed initReady would deadlock Close.
		go func() {
			initWg.Wait()
			close(r.initReady)
		}()
	})
}

// EnsureInitialized triggers lazy worker initialization eagerly and blocks until complete.
func (r *Renderer) EnsureInitialized(ctx context.Context) {
	r.ensureInitialized()

	select {
	case <-r.initReady:
	case <-ctx.Done():
	}
}

// Close shuts down the renderer and cleans up worker resources.
func (r *Renderer) Close() error {
	r.mu.Lock()
	if r.isClosed {
		r.mu.Unlock()
		return nil
	}
	r.isClosed = true
	r.mu.Unlock()

	// Wait for all active tasks to complete.
	r.taskWg.Wait()

	// Ensure initialization is complete before draining pool.
	r.ensureInitialized()
	<-r.initReady

	// Initialization and active tasks are finished; drain the initial queue size.
	for range len(r.pool) {
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
		if instance.peleRenderFn != nil {
			instance.peleRenderFn.Free()
		}
		if instance.pele != nil {
			instance.pele.Free()
		}
		if instance.peleOpts != nil {
			instance.peleOpts.Free()
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
	hasher := xxh3.New()
	_, _ = hasher.WriteString(contentType)
	_, _ = hasher.WriteString(":")
	_, _ = hasher.WriteString(content)
	sum := hasher.Sum128()
	hashBytes := sum.Bytes()
	return hex.EncodeToString(hashBytes[:])[:hashPrefixLength]
}

// GetD2Singleflight returns the shared singleflight group for D2 rendering
func (r *Renderer) GetD2Singleflight() *singleflight.Group {
	return &r.D2Singleflight
}
