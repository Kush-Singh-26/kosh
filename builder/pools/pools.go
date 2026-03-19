package pools

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// BufferPool manages a pool of reusable bytes.Buffer objects
type BufferPool struct {
	pool sync.Pool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}
}

func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > models.MaxBufferSize {
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
			New: func() any {
				return new(strings.Builder)
			},
		},
	}
}

func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

func (p *StringBuilderPool) Put(sb *strings.Builder) {
	if sb.Cap() > models.MaxBufferSize {
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
			New: func() any {
				return bufio.NewWriterSize(nil, models.MaxBufferSize)
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
	return bufio.NewWriterSize(w, models.MaxBufferSize)
}

func (p *BufioWriterPool) Put(bw *bufio.Writer) {
	bw.Reset(nil)
	p.pool.Put(bw)
}

// BufioReaderPool manages a pool of reusable bufio.Reader objects
type BufioReaderPool struct {
	pool sync.Pool
}

func NewBufioReaderPool() *BufioReaderPool {
	return &BufioReaderPool{
		pool: sync.Pool{
			New: func() any {
				return bufio.NewReaderSize(nil, models.MaxBufferSize)
			},
		},
	}
}

func (p *BufioReaderPool) Get(r io.Reader) *bufio.Reader {
	if br := p.pool.Get(); br != nil {
		reader := br.(*bufio.Reader)
		reader.Reset(r)
		return reader
	}
	return bufio.NewReaderSize(r, models.MaxBufferSize)
}

func (p *BufioReaderPool) Put(br *bufio.Reader) {
	br.Reset(nil)
	p.pool.Put(br)
}

// Global shared pool instances
var (
	SharedBufferPool        = NewBufferPool()
	SharedStringBuilderPool = NewStringBuilderPool()
	SharedBufioWriterPool   = NewBufioWriterPool()
	SharedBufioReaderPool   = NewBufioReaderPool()
)

// ByteSlicePool manages a pool of byte slices
type ByteSlicePool struct {
	pool sync.Pool
}

func NewByteSlicePool() *ByteSlicePool {
	return &ByteSlicePool{
		pool: sync.Pool{
			New: func() any {
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
