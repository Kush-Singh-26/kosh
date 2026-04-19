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
	// MaxImageResizeWidth is the maximum width for resized images.
	MaxImageResizeWidth = 1200
	// MaxImageResizeHeight is the maximum height for resized images.
	MaxImageResizeHeight = 1600
	// RGBABytesPerPixel is the number of bytes per RGBA pixel.
	RGBABytesPerPixel = 4
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
func (pool *BufferPool) Get() *bytes.Buffer {
	return pool.pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool.
func (pool *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > models.MaxBufferSize {
		return
	}
	buf.Reset()
	pool.pool.Put(buf)
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
func (pool *StringBuilderPool) Get() *strings.Builder {
	return pool.pool.Get().(*strings.Builder)
}

// Put returns a strings.Builder to the pool.
func (pool *StringBuilderPool) Put(builder *strings.Builder) {
	if builder.Cap() > models.MaxBufferSize {
		return
	}
	builder.Reset()
	pool.pool.Put(builder)
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
func (pool *BufioWriterPool) Get(w io.Writer) *bufio.Writer {
	if bw := pool.pool.Get(); bw != nil {
		writer := bw.(*bufio.Writer)
		writer.Reset(w)
		return writer
	}
	return bufio.NewWriterSize(w, models.MaxBufferSize)
}

// Put returns a bufio.Writer to the pool.
func (pool *BufioWriterPool) Put(bufWriter *bufio.Writer) {
	bufWriter.Reset(nil)
	pool.pool.Put(bufWriter)
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
func (pool *BufioReaderPool) Get(r io.Reader) *bufio.Reader {
	if br := pool.pool.Get(); br != nil {
		reader := br.(*bufio.Reader)
		reader.Reset(r)
		return reader
	}
	return bufio.NewReaderSize(r, models.MaxBufferSize)
}

// Put returns a bufio.Reader to the pool.
func (pool *BufioReaderPool) Put(bufReader *bufio.Reader) {
	bufReader.Reset(nil)
	pool.pool.Put(bufReader)
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
				buffer := make([]byte, 0, byteSlicePoolDefaultCap)
				return &buffer
			},
		},
	}
}

// Get returns a byte slice pointer from the pool.
func (pool *ByteSlicePool) Get() *[]byte {
	return pool.pool.Get().(*[]byte)
}

// Put returns a byte slice to the pool.
func (pool *ByteSlicePool) Put(buffer *[]byte) {
	if buffer == nil || cap(*buffer) > byteSlicePoolMaxCap {
		return
	}
	*buffer = (*buffer)[:0]
	pool.pool.Put(buffer)
}

// SharedByteSlicePool is the shared byte slice pool instance.
var SharedByteSlicePool = NewByteSlicePool()

// IntSlicePool manages a pool of int slices.
type IntSlicePool struct {
	pool sync.Pool // stores *[]int
}

// NewIntSlicePool returns a new IntSlicePool.
func NewIntSlicePool() *IntSlicePool {
	return &IntSlicePool{
		pool: sync.Pool{
			New: func() any {
				buffer := make([]int, 0, 128)
				return &buffer
			},
		},
	}
}

// Get returns an int slice pointer from the pool.
func (pool *IntSlicePool) Get() *[]int {
	return pool.pool.Get().(*[]int)
}

// Put returns an int slice to the pool.
func (pool *IntSlicePool) Put(buffer *[]int) {
	if buffer == nil || cap(*buffer) > 1024*1024 { // max 1M ints (approx 4-8MB)
		return
	}
	*buffer = (*buffer)[:0]
	pool.pool.Put(buffer)
}

// SharedIntSlicePool is the shared int slice pool instance.
var SharedIntSlicePool = NewIntSlicePool()

// RuneSlicePool manages a pool of rune slices.
type RuneSlicePool struct {
	pool sync.Pool // stores *[]rune
}

// NewRuneSlicePool returns a new RuneSlicePool.
func NewRuneSlicePool() *RuneSlicePool {
	return &RuneSlicePool{
		pool: sync.Pool{
			New: func() any {
				buffer := make([]rune, 0, 128)
				return &buffer
			},
		},
	}
}

// Get returns a rune slice pointer from the pool.
func (pool *RuneSlicePool) Get() *[]rune {
	return pool.pool.Get().(*[]rune)
}

// Put returns a rune slice to the pool.
func (pool *RuneSlicePool) Put(buffer *[]rune) {
	if buffer == nil || cap(*buffer) > 1024*1024 { // max 1M runes (approx 4MB)
		return
	}
	*buffer = (*buffer)[:0]
	pool.pool.Put(buffer)
}

// SharedRuneSlicePool is the shared rune slice pool instance.
var SharedRuneSlicePool = NewRuneSlicePool()

// ImageSlicePool manages a pool of byte slices sized for RGBA images.
type ImageSlicePool struct {
	pool sync.Pool // stores *[]byte
}

// NewImageSlicePool returns a new ImageSlicePool.
func NewImageSlicePool() *ImageSlicePool {
	return &ImageSlicePool{
		pool: sync.Pool{
			New: func() any {
				buffer := make([]byte, MaxImageResizeWidth*MaxImageResizeHeight*RGBABytesPerPixel)
				return &buffer
			},
		},
	}
}

// Get returns an image slice pointer from the pool.
func (pool *ImageSlicePool) Get() *[]byte {
	return pool.pool.Get().(*[]byte)
}

// Put returns an image slice to the pool.
func (pool *ImageSlicePool) Put(buffer *[]byte) {
	if buffer == nil || cap(*buffer) > MaxImageResizeWidth*MaxImageResizeHeight*RGBABytesPerPixel*2 {
		return
	}
	pool.pool.Put(buffer)
}

// SharedImageSlicePool is the shared image slice pool instance.
var SharedImageSlicePool = NewImageSlicePool()

// MapStringStringPool manages a pool of map[string]string objects.
type MapStringStringPool struct {
	pool sync.Pool // stores map[string]string
}

// NewMapStringStringPool returns a new MapStringStringPool.
func NewMapStringStringPool() *MapStringStringPool {
	return &MapStringStringPool{
		pool: sync.Pool{
			New: func() any {
				return make(map[string]string)
			},
		},
	}
}

// Get returns a map from the pool.
func (pool *MapStringStringPool) Get() map[string]string {
	return pool.pool.Get().(map[string]string)
}

// Put returns a map to the pool.
func (pool *MapStringStringPool) Put(m map[string]string) {
	if m == nil {
		return
	}
	clear(m)
	pool.pool.Put(m)
}

// MapStringSSRThemePairPool manages a pool of map[string]models.SSRThemePair objects.
type MapStringSSRThemePairPool struct {
	pool sync.Pool // stores map[string]models.SSRThemePair
}

// NewMapStringSSRThemePairPool returns a new MapStringSSRThemePairPool.
func NewMapStringSSRThemePairPool() *MapStringSSRThemePairPool {
	return &MapStringSSRThemePairPool{
		pool: sync.Pool{
			New: func() any {
				return make(map[string]models.SSRThemePair)
			},
		},
	}
}

// Get returns a map from the pool.
func (pool *MapStringSSRThemePairPool) Get() map[string]models.SSRThemePair {
	return pool.pool.Get().(map[string]models.SSRThemePair)
}

// Put returns a map to the pool.
func (pool *MapStringSSRThemePairPool) Put(m map[string]models.SSRThemePair) {
	if m == nil {
		return
	}
	clear(m)
	pool.pool.Put(m)
}

// Global shared map pool instances.
var (
	// SharedMapStringStringPool is the shared map[string]string pool instance.
	SharedMapStringStringPool = NewMapStringStringPool()
	// SharedMapStringSSRThemePairPool is the shared map[string]models.SSRThemePair pool instance.
	SharedMapStringSSRThemePairPool = NewMapStringSSRThemePairPool()
)
