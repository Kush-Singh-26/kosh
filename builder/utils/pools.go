package utils

import (
	"bytes"
	"sync"
)

// BufferPool manages a pool of reusable bytes.Buffer objects
// to reduce memory allocations during high-throughput operations.
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new BufferPool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool, resetting it for reuse.
// If the buffer is too large (> 64KB), it is discarded to prevent memory hoarding.
func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > 64*1024 {
		return // Let GC collect oversized buffers
	}
	buf.Reset()
	p.pool.Put(buf)
}

// Global shared pool instance
var SharedBufferPool = NewBufferPool()
