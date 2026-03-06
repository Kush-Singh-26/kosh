package utils

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
)

const (
	// MaxWorkers is the maximum number of workers in a pool
	MaxWorkers = 32
	// WorkerBufferSize is the channel buffer multiplier
	WorkerBufferSize = 4
)

type WorkerPool[T any] struct {
	workers   int
	ctx       context.Context
	wg        sync.WaitGroup
	taskQueue chan T
	handler   func(T)
	stoppedMu sync.Mutex
	stopped   bool
}

func NewWorkerPool[T any](ctx context.Context, workers int, handler func(T)) *WorkerPool[T] {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > MaxWorkers {
		workers = MaxWorkers
	}
	return &WorkerPool[T]{
		workers:   workers,
		ctx:       ctx,
		taskQueue: make(chan T, workers*WorkerBufferSize),
		handler:   handler,
	}
}

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
			// Recover from panics to prevent worker crashes
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Worker panic recovered", "panic", r)
					}
				}()
				p.handler(task)
			}()
		}
	}
}

func (p *WorkerPool[T]) Submit(task T) {
	p.stoppedMu.Lock()
	defer p.stoppedMu.Unlock()

	if p.stopped {
		return
	}

	select {
	case <-p.ctx.Done():
		return
	case p.taskQueue <- task:
	}
}

func (p *WorkerPool[T]) Stop() {
	p.stoppedMu.Lock()
	if p.stopped {
		p.stoppedMu.Unlock()
		p.wg.Wait()
		return
	}
	p.stopped = true
	close(p.taskQueue)
	p.stoppedMu.Unlock()
	p.wg.Wait()
}
