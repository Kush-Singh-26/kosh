package async

import (
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"

	"context"
	"errors"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of concurrent workers.
type WorkerPool[T any] struct {
	workers   int
	ctx       context.Context
	wg        sync.WaitGroup
	taskQueue chan T
	handler   func(T) error
	stopped   atomic.Bool
	scheduler scheduler.BuildScheduler
	taskType  scheduler.TaskType
	errs      []error
	mu        sync.Mutex
}

// NewWorkerPool constructs a worker pool with bounded concurrency.
func NewWorkerPool[T any](ctx context.Context, workers int, handler func(T) error) *WorkerPool[T] {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > models.MaxWorkers {
		workers = models.MaxWorkers
	}
	return &WorkerPool[T]{
		workers:   workers,
		ctx:       ctx,
		taskQueue: make(chan T, workers*models.WorkerBufferSize),
		handler:   handler,
	}
}

// WithScheduler attaches a global scheduler to the pool.
func (p *WorkerPool[T]) WithScheduler(s scheduler.BuildScheduler, t scheduler.TaskType) *WorkerPool[T] {
	p.scheduler = s
	p.taskType = t
	return p
}

// Start launches the worker goroutines.
func (p *WorkerPool[T]) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

func (p *WorkerPool[T]) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}

			// If a scheduler is attached, acquire tokens before work
			if p.scheduler != nil {
				if err := p.scheduler.Acquire(p.ctx, p.taskType); err != nil {
					return
				}
			}

			// Recover from panics to prevent worker crashes
			func() {
				defer func() {
					if p.scheduler != nil {
						p.scheduler.Release(p.taskType)
					}
					if r := recover(); r != nil {
						slog.Error("Worker panic recovered", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				if err := p.handler(task); err != nil {
					p.mu.Lock()
					p.errs = append(p.errs, err)
					p.mu.Unlock()
				}
			}()
		}
	}
}

// Submit enqueues a task for processing.
func (p *WorkerPool[T]) Submit(task T) {
	if p.stopped.Load() {
		return
	}

	select {
	case <-p.ctx.Done():
		return
	case p.taskQueue <- task:
	}
}

// Stop closes the queue and waits for all workers to finish.
func (p *WorkerPool[T]) Stop() error {
	if !p.stopped.CompareAndSwap(false, true) {
		p.wg.Wait()
		return errors.Join(p.errs...)
	}
	close(p.taskQueue)
	p.wg.Wait()
	return errors.Join(p.errs...)
}
