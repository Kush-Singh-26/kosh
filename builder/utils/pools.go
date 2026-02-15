package utils

import (
	"bufio"
	"bytes"
	"io"
	"strings"
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
// If the buffer is too large, it is discarded to prevent memory hoarding.
func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > MaxBufferSize {
		return
	}
	buf.Reset()
	p.pool.Put(buf)
}

// StringBuilderPool manages a pool of reusable strings.Builder objects
type StringBuilderPool struct {
	pool sync.Pool
}

// NewStringBuilderPool creates a new StringBuilderPool
func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				sb := new(strings.Builder)
				sb.Grow(256)
				return sb
			},
		},
	}
}

// Get retrieves a strings.Builder from the pool
func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

// Put returns a strings.Builder to the pool, resetting it for reuse
func (p *StringBuilderPool) Put(sb *strings.Builder) {
	if sb.Cap() > MaxBufferSize {
		return
	}
	sb.Reset()
	p.pool.Put(sb)
}

// BufioWriterPool manages a pool of reusable bufio.Writer objects
type BufioWriterPool struct {
	pool sync.Pool
}

// NewBufioWriterPool creates a new BufioWriterPool
func NewBufioWriterPool() *BufioWriterPool {
	return &BufioWriterPool{
		pool: sync.Pool{
			New: func() interface{} {
				return nil
			},
		},
	}
}

// Get retrieves a bufio.Writer from the pool, configured with the target writer
func (p *BufioWriterPool) Get(w io.Writer) *bufio.Writer {
	if bw := p.pool.Get(); bw != nil {
		writer := bw.(*bufio.Writer)
		writer.Reset(w)
		return writer
	}
	return bufio.NewWriterSize(w, MaxBufferSize)
}

// Put returns a bufio.Writer to the pool
func (p *BufioWriterPool) Put(bw *bufio.Writer) {
	p.pool.Put(bw)
}

// Global shared pool instances
var (
	SharedBufferPool        = NewBufferPool()
	SharedStringBuilderPool = NewStringBuilderPool()
	SharedBufioWriterPool   = NewBufioWriterPool()
)
