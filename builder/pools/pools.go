package pools

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

const (
	bytesPerKiB             = 1024
	bytesPerMiB             = 1024 * bytesPerKiB
	byteSlicePoolDefaultCap = 10 * bytesPerKiB
	byteSlicePoolMaxCap     = 5 * bytesPerMiB
)

// BufferPool manages a pool of reusable bytes.Buffer objects.
type BufferPool struct {
	pool sync.Pool // stores *bytes.Buffer
}

// NewBufferPool returns a new BufferPool.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}
}

// Get returns a buffer from the pool.
func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool.
func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > models.MaxBufferSize {
		return
	}
	buf.Reset()
	p.pool.Put(buf)
}

// StringBuilderPool manages a pool of reusable strings.Builder objects.
type StringBuilderPool struct {
	pool sync.Pool // stores *strings.Builder
}

// NewStringBuilderPool returns a new StringBuilderPool.
func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() any {
				return new(strings.Builder)
			},
		},
	}
}

// Get returns a strings.Builder from the pool.
func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

// Put returns a strings.Builder to the pool.
func (p *StringBuilderPool) Put(sb *strings.Builder) {
	if sb.Cap() > models.MaxBufferSize {
		return
	}
	sb.Reset()
	p.pool.Put(sb)
}

// BufioWriterPool manages a pool of reusable bufio.Writer objects.
type BufioWriterPool struct {
	pool sync.Pool // stores *bufio.Writer
}

// NewBufioWriterPool returns a new BufioWriterPool.
func NewBufioWriterPool() *BufioWriterPool {
	return &BufioWriterPool{
		pool: sync.Pool{
			New: func() any {
				return bufio.NewWriterSize(nil, models.MaxBufferSize)
			},
		},
	}
}

// Get returns a bufio.Writer for the provided writer.
func (p *BufioWriterPool) Get(w io.Writer) *bufio.Writer {
	if bw := p.pool.Get(); bw != nil {
		writer := bw.(*bufio.Writer)
		writer.Reset(w)
		return writer
	}
	return bufio.NewWriterSize(w, models.MaxBufferSize)
}

// Put returns a bufio.Writer to the pool.
func (p *BufioWriterPool) Put(bw *bufio.Writer) {
	bw.Reset(nil)
	p.pool.Put(bw)
}

// BufioReaderPool manages a pool of reusable bufio.Reader objects.
type BufioReaderPool struct {
	pool sync.Pool // stores *bufio.Reader
}

// NewBufioReaderPool returns a new BufioReaderPool.
func NewBufioReaderPool() *BufioReaderPool {
	return &BufioReaderPool{
		pool: sync.Pool{
			New: func() any {
				return bufio.NewReaderSize(nil, models.MaxBufferSize)
			},
		},
	}
}

// Get returns a bufio.Reader for the provided reader.
func (p *BufioReaderPool) Get(r io.Reader) *bufio.Reader {
	if br := p.pool.Get(); br != nil {
		reader := br.(*bufio.Reader)
		reader.Reset(r)
		return reader
	}
	return bufio.NewReaderSize(r, models.MaxBufferSize)
}

// Put returns a bufio.Reader to the pool.
func (p *BufioReaderPool) Put(br *bufio.Reader) {
	br.Reset(nil)
	p.pool.Put(br)
}

// Global shared pool instances.
var (
	// SharedBufferPool is the shared buffer pool instance.
	SharedBufferPool = NewBufferPool()
	// SharedStringBuilderPool is the shared strings.Builder pool instance.
	SharedStringBuilderPool = NewStringBuilderPool()
	// SharedBufioWriterPool is the shared bufio.Writer pool instance.
	SharedBufioWriterPool = NewBufioWriterPool()
	// SharedBufioReaderPool is the shared bufio.Reader pool instance.
	SharedBufioReaderPool = NewBufioReaderPool()
)

// ByteSlicePool manages a pool of byte slices.
type ByteSlicePool struct {
	pool sync.Pool // stores *[]byte
}

// NewByteSlicePool returns a new ByteSlicePool.
func NewByteSlicePool() *ByteSlicePool {
	return &ByteSlicePool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, 0, byteSlicePoolDefaultCap)
				return &b
			},
		},
	}
}

// Get returns a byte slice pointer from the pool.
func (p *ByteSlicePool) Get() *[]byte {
	return p.pool.Get().(*[]byte)
}

// Put returns a byte slice to the pool.
func (p *ByteSlicePool) Put(b *[]byte) {
	if b == nil || cap(*b) > byteSlicePoolMaxCap {
		return
	}
	*b = (*b)[:0]
	p.pool.Put(b)
}

// SharedByteSlicePool is the shared byte slice pool instance.
var SharedByteSlicePool = NewByteSlicePool()
