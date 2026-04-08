package assets

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/errgroup"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

// webpBufferPool stores *bytes.Buffer instances for WebP encoding.
var webpBufferPool = sync.Pool{
	New: func() any {
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

// Size reports the current in-memory cache size in bytes.
func (c *imageCache) Size() int64 { return c.size.Load() }

var (
	globalImageCache     *imageCache
	globalImageCacheOnce sync.Once
)

// GetImageCache returns the global in-memory image cache.
func GetImageCache() *imageCache {
	globalImageCacheOnce.Do(func() {
		globalImageCache = newImageCache(400, 100*1024*1024)
	})
	return globalImageCache
}

// convertedImagePaths stores string->string mappings of original to WebP paths.
var convertedImagePaths sync.Map

// RecordConvertedImage stores a mapping from original to WebP path.
func RecordConvertedImage(originalDst, webpDst string) {
	convertedImagePaths.Store(originalDst, webpDst)
}

// GetConvertedImages returns a snapshot of recorded image conversions.
func GetConvertedImages() map[string]string {
	result := make(map[string]string)
	convertedImagePaths.Range(func(key, value any) bool {
		result[key.(string)] = value.(string)
		return true
	})
	return result
}

// ResetConvertedImages clears the recorded image conversion map.
func ResetConvertedImages() {
	convertedImagePaths = sync.Map{}
}

// keyBufPool stores *[]byte buffers for hash key construction.
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

// Load returns the current value.
func (a *atomicInt64) Load() int64 { return atomic.LoadInt64(&a.v) }

// Add increments the value by delta.
func (a *atomicInt64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }

var imageCacheWriter struct {
	ch   chan imageCacheEntry
	once sync.Once
}

func initImageCacheWriter() {
	imageCacheWriter.once.Do(func() {
		imageCacheWriter.ch = make(chan imageCacheEntry, 2048)
		async.FireAndForget(context.Background(), slog.Default(), "image cache writer", func() error {
			for entry := range imageCacheWriter.ch {
				_ = os.MkdirAll(filepath.Dir(entry.path), 0755)
				if err := os.WriteFile(entry.path, entry.data, 0644); err != nil {
					slog.Warn("Failed to write image cache file", "path", entry.path, "error", err)
				}
			}
			return nil
		})
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

// ErrCacheMiss is returned when an image is not in the disk cache.
var ErrCacheMiss = errors.New("image not in disk cache")

// CopyFromDiskCacheOptions bundles parameters for CopyFromDiskCache.
type CopyFromDiskCacheOptions struct {
	SrcFs        afero.Fs
	Sink         fspkg.ArtifactSink
	RelPath      string
	SrcPath      string
	DstPath      string
	CacheDir     string
	SrcInfo      fs.FileInfo
	Metrics      ImageMetrics
	OnWrite      func(string)
	KeepOriginal bool
	MuteMetrics  bool
}

// CopyFromDiskCache checks the on-disk WebP cache for an image and, if found,
// writes it to the sink and registers it in the converted-images map.
// Returns nil on cache hit, ErrCacheMiss on miss, or a real error.
// Called from the asset discovery goroutine to speed up image registration
// before background workers process the remaining cache-miss images.
func CopyFromDiskCache(opts CopyFromDiskCacheOptions) error {
	cacheFs := afero.NewOsFs()
	memKey := imageCacheKey{
		path:    opts.SrcPath,
		size:    opts.SrcInfo.Size(),
		modTime: opts.SrcInfo.ModTime().UnixNano(),
	}
	hash := getImageHash(memKey)
	cacheFile := filepath.Join(opts.CacheDir, hash+".webp")

	cachedData, err := afero.ReadFile(cacheFs, cacheFile)
	if err != nil {
		return ErrCacheMiss
	}

	if err := opts.Sink.WriteFile(opts.DstPath, cachedData); err != nil {
		return err
	}

	if opts.Metrics != nil && !opts.MuteMetrics {
		opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cachedData)))
		opts.Metrics.IncrementAssetsProcessed()
	}

	relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(opts.RelPath), "/")
	relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
	registerImageVariants(relSrc, relDst)

	if opts.OnWrite != nil {
		opts.OnWrite(opts.DstPath)
	}

	// If keepOriginal is requested, also copy the source file to its destination
	if opts.KeepOriginal {
		ext := strings.ToLower(filepath.Ext(opts.SrcPath))
		origDst := strings.TrimSuffix(opts.DstPath, filepath.Ext(opts.DstPath)) + ext
		_ = fspkg.CopyFileVFS(fspkg.CopyFileOptions{
			SrcFs:   opts.SrcFs,
			Sink:    opts.Sink,
			SrcPath: opts.SrcPath,
			DstPath: origDst,
			ModTime: opts.SrcInfo.ModTime().UnixNano(),
			OnWrite: opts.OnWrite,
		})
	}

	return nil
}

// registerImageVariants records an image conversion mapping in all common
// path forms so that both "/static/foo.png" and "static/foo.png" references
// in HTML can be found during the rewrite phase. Also registers case variants
// to handle case-insensitive file references (e.g. "Foo.PNG" vs "foo.png").
func registerImageVariants(srcPath, webpPath string) {
	RecordConvertedImage(srcPath, webpPath)
	if strings.HasPrefix(srcPath, "/") {
		RecordConvertedImage(strings.TrimPrefix(srcPath, "/"), webpPath)
	} else {
		RecordConvertedImage("/"+srcPath, webpPath)
	}
	// Case variants to catch case-insensitive references in raw HTML
	lowerSrc := strings.ToLower(srcPath)
	if lowerSrc != srcPath {
		RecordConvertedImage(lowerSrc, webpPath)
		if strings.HasPrefix(lowerSrc, "/") {
			RecordConvertedImage(strings.TrimPrefix(lowerSrc, "/"), webpPath)
		} else {
			RecordConvertedImage("/"+lowerSrc, webpPath)
		}
	}
}

// CleanupOriginalImages removes source image files (.png/.jpg/.jpeg) from the
// output directory when a corresponding .webp file exists. It uses the known
// conversion map, eliminating expensive filesystem sweeps.
func CleanupOriginalImages(outputDir string) {
	converted := GetConvertedImages()
	if len(converted) == 0 {
		return
	}

	// Critical .png files that must be preserved
	criticalPNGs := map[string]bool{
		"logo.png":     true,
		"icon-192.png": true,
		"icon-512.png": true,
	}

	// Determine exactly which source images to delete based on the converted map
	var toDelete []string
	for origRelPath := range converted {
		lower := strings.ToLower(origRelPath)
		if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") {
			continue
		}

		base := filepath.Base(origRelPath)
		if criticalPNGs[strings.ToLower(base)] {
			continue
		}

		// Map the relative path to the physical output dir
		absPath := filepath.Join(outputDir, strings.TrimPrefix(filepath.ToSlash(origRelPath), "/"))
		toDelete = append(toDelete, absPath)
	}

	if len(toDelete) == 0 {
		return
	}

	// Delete in parallel
	var deleted atomic.Int64
	numWorkers := runtime.NumCPU()
	if numWorkers > len(toDelete) {
		numWorkers = len(toDelete)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(numWorkers)

	for _, fullPath := range toDelete {
		p := fullPath
		g.Go(func() error {
			if err := os.Remove(p); err == nil {
				deleted.Add(1)
			} else if !os.IsNotExist(err) {
				return err // Propagate real errors instead of swallowing them
			}
			return nil
		})
	}
	_ = g.Wait()

	if d := deleted.Load(); d > 0 {
		slog.Info("Cleaned up original images", "deleted", d)
	}
}
