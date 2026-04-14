//go:build !wasm

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

const (
	webpBufferPoolCap        = 256 * 1024
	imageCacheEntryOverhead  = 128
	defaultImageCacheItems   = 400
	defaultImageCacheMaxSize = 100 * 1024 * 1024
	keyBufCap                = 512
	imageCacheWriterBuffer   = 2048
	imageCacheDirMode        = 0755
	imageCacheFileMode       = 0644
	strconvBase10            = 10
)

var criticalOriginalPNGs = map[string]struct{}{
	"logo.png":     {},
	"icon-192.png": {},
	"icon-512.png": {},
}

// webpBufferPool stores *bytes.Buffer instances for WebP encoding.
var webpBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, webpBufferPoolCap))
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
	cacheInstance := &imageCache{
		capacity: maxBytes,
	}

	onEvict := func(key imageCacheKey, value []byte) {
		overhead := imageCacheEntryOverhead + len(key.path)
		cacheInstance.size.Add(-int64(cap(value) + overhead))
	}

	cache, _ := lru.NewWithEvict[imageCacheKey, []byte](maxItems, onEvict)
	cacheInstance.cache = cache

	return cacheInstance
}

func (cache *imageCache) get(key imageCacheKey) ([]byte, bool) {
	return cache.cache.Get(key)
}

func (cache *imageCache) set(key imageCacheKey, data []byte) {
	overhead := imageCacheEntryOverhead + len(key.path)
	itemSize := cap(data) + overhead

	cache.size.Add(int64(itemSize))

	for cache.size.Load() > cache.capacity && cache.cache.Len() > 0 {
		_, _, ok := cache.cache.RemoveOldest()
		if !ok {
			break
		}
	}

	cache.cache.Add(key, data)
}

// Size reports the current in-memory cache size in bytes.
func (cache *imageCache) Size() int64 { return cache.size.Load() }

var (
	globalImageCache     *imageCache
	globalImageCacheOnce sync.Once
)

// GetImageCache returns the global in-memory image cache.
func GetImageCache() *imageCache {
	globalImageCacheOnce.Do(func() {
		globalImageCache = newImageCache(defaultImageCacheItems, defaultImageCacheMaxSize)
	})
	return globalImageCache
}

// convertedImagePaths stores string->string mappings of original to WebP paths.
var convertedImagePaths sync.Map

// RecordConvertedImage stores a mapping from original to WebP path.
func RecordConvertedImage(originalDestination, webpDestination string) {
	convertedImagePaths.Store(originalDestination, webpDestination)
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
		buffer := make([]byte, 0, keyBufCap)
		return &buffer
	},
}

func getImageHash(key imageCacheKey) string {
	bufferPointer := keyBufPool.Get().(*[]byte)
	buffer := (*bufferPointer)[:0]
	defer func() {
		*bufferPointer = buffer
		keyBufPool.Put(bufferPointer)
	}()

	buffer = append(buffer, key.path...)
	buffer = strconv.AppendInt(buffer, key.size, strconvBase10)
	buffer = strconv.AppendInt(buffer, key.modTime, strconvBase10)

	hash := xxh3.Hash128(buffer)
	resultBytes := hash.Bytes()
	return hex.EncodeToString(resultBytes[:])
}

type atomicInt64 struct{ v int64 }

// Load returns the current value.
func (a *atomicInt64) Load() int64 { return atomic.LoadInt64(&a.v) }

// Add increments the value by delta.
func (a *atomicInt64) Add(delta int64) { atomic.AddInt64(&a.v, delta) }

var imageCacheWriter struct {
	channel chan imageCacheEntry
	once    sync.Once
}

func initImageCacheWriter() {
	imageCacheWriter.once.Do(func() {
		imageCacheWriter.channel = make(chan imageCacheEntry, imageCacheWriterBuffer)
		async.FireAndForget(context.Background(), slog.Default(), "image cache writer", func() error {
			for cacheEntry := range imageCacheWriter.channel {
				_ = os.MkdirAll(filepath.Dir(cacheEntry.path), imageCacheDirMode)
				if err := os.WriteFile(cacheEntry.path, cacheEntry.data, imageCacheFileMode); err != nil {
					slog.Warn("Failed to write image cache file", "path", cacheEntry.path, "error", err)
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
	case imageCacheWriter.channel <- imageCacheEntry{path: path, data: dataCopy}:
	default:
		_ = os.MkdirAll(filepath.Dir(path), imageCacheDirMode)
		_ = os.WriteFile(path, dataCopy, imageCacheFileMode)
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
func CopyFromDiskCache(options CopyFromDiskCacheOptions) error {
	cacheFs := afero.NewOsFs()
	memoryCacheKey := imageCacheKey{
		path:    options.SrcPath,
		size:    options.SrcInfo.Size(),
		modTime: options.SrcInfo.ModTime().UnixNano(),
	}
	hash := getImageHash(memoryCacheKey)
	cacheFile := filepath.Join(options.CacheDir, hash+".webp")

	cachedData, err := afero.ReadFile(cacheFs, cacheFile)
	if err != nil {
		return ErrCacheMiss
	}

	if err := options.Sink.WriteFile(options.DstPath, cachedData); err != nil {
		return err
	}

	if options.Metrics != nil && !options.MuteMetrics {
		options.Metrics.RecordImageOptimization(options.SrcInfo.Size(), int64(len(cachedData)))
		options.Metrics.IncrementAssetsProcessed()
	}

	relativeSource := "/" + strings.TrimPrefix(filepath.ToSlash(options.RelPath), "/")
	relativeDestination := relativeSource[:len(relativeSource)-len(filepath.Ext(relativeSource))] + ".webp"
	registerImageVariants(relativeSource, relativeDestination)

	if options.OnWrite != nil {
		options.OnWrite(options.DstPath)
	}

	// If keepOriginal is requested, also copy the source file to its destination
	if options.KeepOriginal {
		extension := strings.ToLower(filepath.Ext(options.SrcPath))
		originalDestination := strings.TrimSuffix(options.DstPath, filepath.Ext(options.DstPath)) + extension
		_ = fspkg.CopyFileVFS(fspkg.CopyFileOptions{
			SrcFs:   options.SrcFs,
			Sink:    options.Sink,
			SrcPath: options.SrcPath,
			DstPath: originalDestination,
			ModTime: options.SrcInfo.ModTime().UnixNano(),
			OnWrite: options.OnWrite,
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

func shouldPreserveOriginal(name string) bool {
	_, ok := criticalOriginalPNGs[strings.ToLower(name)]
	return ok
}

func collectOriginalsToDelete(outputDir string, converted map[string]string) []string {
	if len(converted) == 0 {
		return nil
	}

	keys := make([]string, 0, len(converted))
	for k := range converted {
		keys = append(keys, k)
	}

	toDelete := make([]string, 0, len(keys))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())

	for _, k := range keys {
		originalPath := k
		g.Go(func() error {
			lower := strings.ToLower(originalPath)
			if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") {
				return nil
			}

			base := filepath.Base(originalPath)
			if shouldPreserveOriginal(base) {
				return nil
			}

			absolutePath := filepath.Join(outputDir, strings.TrimPrefix(filepath.ToSlash(originalPath), "/"))
			mu.Lock()
			toDelete = append(toDelete, absolutePath)
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return toDelete
}

func deleteOriginals(paths []string) int64 {
	if len(paths) == 0 {
		return 0
	}

	var deleted atomic.Int64
	numWorkers := runtime.NumCPU()
	if numWorkers > len(paths) {
		numWorkers = len(paths)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	errorGroup, _ := errgroup.WithContext(context.Background())
	errorGroup.SetLimit(numWorkers)

	for _, fullPath := range paths {
		path := fullPath
		errorGroup.Go(func() error {
			if err := os.Remove(path); err == nil {
				deleted.Add(1)
			} else if !os.IsNotExist(err) {
				return err // Propagate real errors instead of swallowing them
			}
			return nil
		})
	}
	_ = errorGroup.Wait() // Best-effort cleanup; errors are already filtered above.

	return deleted.Load()
}

// CleanupOriginalImages removes source image files (.png/.jpg/.jpeg) from the
// output directory when a corresponding .webp file exists. It uses the known
// conversion map, eliminating expensive filesystem sweeps.
func CleanupOriginalImages(outputDir string) {
	converted := GetConvertedImages()
	if len(converted) == 0 {
		return
	}
	toDelete := collectOriginalsToDelete(outputDir, converted)
	deleted := deleteOriginals(toDelete)
	if deleted > 0 {
		slog.Info("Cleaned up original images", "deleted", deleted)
	}
}
