package fs

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/zeebo/xxh3"
)

var webpBufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 256*1024))
	},
}

type imageCacheKey struct {
	path    string
	size    int64
	modTime int64
}

type imageCache struct {
	cache    *lru.Cache[imageCacheKey, []byte]
	size     atomicInt64
	capacity int64
}

type imageCacheEntry struct {
	path string
	data []byte
}

func newImageCache(maxItems int, maxBytes int64) *imageCache {
	ic := &imageCache{
		capacity: maxBytes,
	}

	onEvict := func(key imageCacheKey, value []byte) {
		overhead := 128 + len(key.path)
		ic.size.Add(-int64(cap(value) + overhead))
	}

	c, _ := lru.NewWithEvict[imageCacheKey, []byte](maxItems, onEvict)
	ic.cache = c

	return ic
}

func (c *imageCache) get(key imageCacheKey) ([]byte, bool) {
	return c.cache.Get(key)
}

func (c *imageCache) set(key imageCacheKey, data []byte) {
	overhead := 128 + len(key.path)
	itemSize := cap(data) + overhead

	c.size.Add(int64(itemSize))

	for c.size.Load() > c.capacity && c.cache.Len() > 0 {
		_, _, ok := c.cache.RemoveOldest()
		if !ok {
			break
		}
	}

	c.cache.Add(key, data)
}

func (c *imageCache) Size() int64 { return c.size.Load() }

var (
	globalImageCache     *imageCache
	globalImageCacheOnce sync.Once
)

func GetImageCache() *imageCache {
	globalImageCacheOnce.Do(func() {
		globalImageCache = newImageCache(400, 100*1024*1024)
	})
	return globalImageCache
}

var convertedImagePaths sync.Map

func RecordConvertedImage(originalDst, webpDst string) {
	convertedImagePaths.Store(originalDst, webpDst)
}

func GetConvertedImages() map[string]string {
	result := make(map[string]string)
	convertedImagePaths.Range(func(key, value any) bool {
		result[key.(string)] = value.(string)
		return true
	})
	return result
}

func ResetConvertedImages() {
	convertedImagePaths = sync.Map{}
}

var keyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

func getImageHash(key imageCacheKey) string {
	bufPtr := keyBufPool.Get().(*[]byte)
	buf := (*bufPtr)[:0]
	defer func() {
		*bufPtr = buf
		keyBufPool.Put(bufPtr)
	}()

	buf = append(buf, key.path...)
	buf = strconv.AppendInt(buf, key.size, 10)
	buf = strconv.AppendInt(buf, key.modTime, 10)

	h := xxh3.Hash128(buf)
	res := h.Bytes()
	return hex.EncodeToString(res[:])
}

type atomicInt64 struct{ v int64 }

func (a *atomicInt64) Load() int64     { return atomic.LoadInt64(&a.v) }
func (a *atomicInt64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }

var imageCacheWriter struct {
	ch   chan imageCacheEntry
	once sync.Once
}

func initImageCacheWriter() {
	imageCacheWriter.once.Do(func() {
		imageCacheWriter.ch = make(chan imageCacheEntry, 2048)
		go func() {
			for entry := range imageCacheWriter.ch {
				_ = os.MkdirAll(filepath.Dir(entry.path), 0755)
				if err := os.WriteFile(entry.path, entry.data, 0644); err != nil {
					slog.Warn("Failed to write image cache file", "path", entry.path, "error", err)
				}
			}
		}()
	})
}

func queueImageCacheWrite(path string, data []byte, isCloned bool) {
	initImageCacheWriter()

	var dataCopy []byte
	if isCloned {
		dataCopy = data
	} else {
		dataCopy = make([]byte, len(data))
		copy(dataCopy, data)
	}

	select {
	case imageCacheWriter.ch <- imageCacheEntry{path: path, data: dataCopy}:
	default:
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, dataCopy, 0644)
	}
}
