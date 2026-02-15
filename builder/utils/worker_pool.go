package utils

import (
	"context"
	"runtime"
	"sync"
)

const (
	MaxWorkers       = 32
	WorkerBufferSize = 4
)

type WorkerPool[T any] struct {
	workers   int
	ctx       context.Context
	wg        sync.WaitGroup
	taskQueue chan T
	handler   func(T)
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
			p.handler(task)
		}
	}
}

func (p *WorkerPool[T]) Submit(task T) {
	select {
	case <-p.ctx.Done():
		return
	case p.taskQueue <- task:
	}
}

func (p *WorkerPool[T]) Stop() {
	close(p.taskQueue)
	p.wg.Wait()
}
