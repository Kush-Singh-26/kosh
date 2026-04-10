// Package native provides a native Go renderer for D2 diagrams and LaTeX math.
package native

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"

	"github.com/fastschema/qjs"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/singleflight"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const (
	minWorkers           = 1
	defaultMathBatchSize = 16
	mathQueueBufferSize  = 2048
	mathBatchTimeout     = 2 * time.Millisecond
	mathBatchErrTimeout  = 10 * time.Millisecond

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

// Renderer manages a pool of native rendering instances for concurrency
type Renderer struct {
	pool           chan *instance
	rulerPool      sync.Pool // stores *textmeasure.Ruler instances
	numWorkers     int
	mathBatchSize  int
	initOnce       sync.Once
	katexBytecode  []byte
	initReady      chan struct{}
	taskWg         sync.WaitGroup
	mu             sync.Mutex // protects closed during pool initialization
	closed         bool
	D2Singleflight singleflight.Group // Shared group to deduplicate D2 diagram rendering across posts
	scheduler      scheduler.BuildScheduler
	mathQueue      chan mathRequest
}

type mathRequest struct {
	expr models.MathExpression
	res  chan string
	err  chan error
}

// RendererOption configures a Renderer.
type RendererOption func(*Renderer)

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

// WithMathBatchSize sets the batch size for math rendering.
func WithMathBatchSize(batchSize int) RendererOption {
	return func(r *Renderer) {
		if batchSize > 0 {
			r.mathBatchSize = batchSize
		}
	}
}

// New creates a new Renderer - workers are lazy-initialized
func New(opts ...RendererOption) *Renderer {
	numWorkers := max(runtime.NumCPU(), minWorkers)

	r := &Renderer{
		pool: make(chan *instance, numWorkers),
		rulerPool: sync.Pool{
			New: func() any {
				ruler, _ := textmeasure.NewRuler()
				return ruler
			},
		},
		numWorkers:    numWorkers,
		mathBatchSize: defaultMathBatchSize,
		initReady:     make(chan struct{}),
		scheduler:     nil, // Must be set via WithScheduler option
		mathQueue:     make(chan mathRequest, mathQueueBufferSize),
	}

	for _, opt := range opts {
		opt(r)
	}

	// Re-create pool with correct size if workers were changed
	if r.numWorkers != numWorkers {
		r.pool = make(chan *instance, r.numWorkers)
	}

	// Start math batcher workers
	for i := 0; i < r.numWorkers; i++ {
		async.FireAndForget(context.Background(), slog.Default(), "math batch worker", func() error {
			r.mathBatchWorker()
			return nil
		})
	}

	return r
}

func (r *Renderer) mathBatchWorker() {
	for req := range r.mathQueue {
		// Collect a small batch
		batch := []mathRequest{req}
		timeout := time.After(mathBatchTimeout)

	loop:
		for len(batch) < r.mathBatchSize {
			select {
			case next, ok := <-r.mathQueue:
				if !ok {
					break loop
				}
				batch = append(batch, next)
			case <-timeout:
				break loop
			}
		}

		exprs := make([]models.MathExpression, len(batch))
		for idx, item := range batch {
			exprs[idx] = item.expr
		}

		results, err := r.RenderMathBatch(context.Background(), exprs)
		if err != nil {
			for _, b := range batch {
				select {
				case b.err <- err:
				case <-time.After(mathBatchErrTimeout):
				}
			}
			continue
		}

		for i, res := range results {
			batch[i].res <- res
		}
	}
}

func (r *Renderer) validateBytecode(bytecode []byte) ([]byte, bool) {
	if len(bytecode) < bytecodeHeaderSize {
		return nil, false
	}
	if string(bytecode[0:bytecodeMagicSize]) != bytecodeMagic {
		return nil, false
	}
	// Check hash of source JS
	sourceHash := binary.LittleEndian.Uint64(bytecode[bytecodeSourceHashStart:bytecodeSourceHashEnd])
	actualHash := xxh3.Hash([]byte(katexJS))
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
func (r *Renderer) ensureInitialized() {
	r.initOnce.Do(func() {
		slog.Info("Initializing QuickJS Renderer Pool", "workers", r.numWorkers)

		// Validate embedded bytecode
		if bytecode, ok := r.validateBytecode(katexBytecode); ok {
			slog.Info("Using validated KaTeX bytecode", "size", len(bytecode))
			r.katexBytecode = bytecode
		} else {
			if len(katexBytecode) > 0 {
				slog.Warn("KaTeX bytecode validation failed (stale or invalid), recompiling...")
			} else {
				slog.Info("No KaTeX bytecode found, compiling from source...")
			}

			jsRuntime, err := qjs.New(qjs.Option{
				MaxExecutionTime: maxExecutionTimeMs,
			})
			if err == nil {
				bytecode, err := jsRuntime.Compile("katex.min.js", qjs.Code(katexJS))
				if err == nil {
					r.katexBytecode = bytecode
				} else {
					slog.Warn("Failed to compile KaTeX to bytecode", "error", err)
				}
				jsRuntime.Close()
			} else {
				slog.Warn("Failed to create temporary runtime for compilation", "error", err)
			}
		}

		// Start workers in background without blocking
		var initWg sync.WaitGroup
		initWg.Add(r.numWorkers)
		for i := 0; i < r.numWorkers; i++ {
			async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
				Ctx:       context.Background(),
				Logger:    slog.Default(),
				Operation: "native renderer init worker",
				Fn: func() error {
					instance := &instance{}
					instance.ensureInitialized(r.katexBytecode)

					r.mu.Lock()
					isClosed := r.closed
					r.mu.Unlock()

					if !isClosed {
						r.pool <- instance
					}
					return nil
				},
				Cleanup: initWg.Done,
			})
		}

		// Close initReady in background when all workers are started
		async.FireAndForget(context.Background(), slog.Default(), "native renderer init", func() error {
			initWg.Wait()
			close(r.initReady)
			return nil
		})
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
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	// Stop math batchers
	close(r.mathQueue)

	// Wait for all active tasks to complete.
	r.taskWg.Wait()

	// Ensure initialization is complete before draining pool.
	// If it was never started, ensureInitialized will start it and we wait.
	// This ensures we don't leave goroutines or runtimes dangling.
	r.ensureInitialized()
	<-r.initReady

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
	hasher := xxh3.New()
	_, _ = hasher.WriteString(contentType + ":" + content)
	sum := hasher.Sum128()
	hashBytes := sum.Bytes()
	return hex.EncodeToString(hashBytes[:])[:hashPrefixLength]
}

// GetD2Singleflight returns the shared singleflight group for D2 rendering
func (r *Renderer) GetD2Singleflight() *singleflight.Group {
	return &r.D2Singleflight
}
