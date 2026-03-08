package utils

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"sync"
)

// BufferPool manages a pool of reusable bytes.Buffer objects
type BufferPool struct {
	pool sync.Pool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

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

func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(strings.Builder)
			},
		},
	}
}

func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

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

func NewBufioWriterPool() *BufioWriterPool {
	return &BufioWriterPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bufio.NewWriterSize(nil, MaxBufferSize)
			},
		},
	}
}

func (p *BufioWriterPool) Get(w io.Writer) *bufio.Writer {
	if bw := p.pool.Get(); bw != nil {
		writer := bw.(*bufio.Writer)
		writer.Reset(w)
		return writer
	}
	return bufio.NewWriterSize(w, MaxBufferSize)
}

func (p *BufioWriterPool) Put(bw *bufio.Writer) {
	if bw.Size() > MaxBufferSize {
		return // Don't return oversized writers to the pool
	}
	p.pool.Put(bw)
}

// Global shared pool instances
var (
	SharedBufferPool      = NewBufferPool()
	SharedStringBuilderPool = NewStringBuilderPool()
	SharedBufioWriterPool = NewBufioWriterPool()
)

// ByteSlicePool manages a pool of byte slices
type ByteSlicePool struct {
	pool sync.Pool
}

func NewByteSlicePool() *ByteSlicePool {
	return &ByteSlicePool{
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, 0, 1024*10) // 10KB
				return &b
			},
		},
	}
}

func (p *ByteSlicePool) Get() *[]byte {
	return p.pool.Get().(*[]byte)
}

func (p *ByteSlicePool) Put(b *[]byte) {
	if b == nil || cap(*b) > 5*1024*1024 {
		return
	}
	*b = (*b)[:0]
	p.pool.Put(b)
}

var SharedByteSlicePool = NewByteSlicePool()
