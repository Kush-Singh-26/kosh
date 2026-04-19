//go:build !wasm

package assets

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
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

// ImageCache is an LRU cache for processed images.
type ImageCache struct {
	cache    *lru.Cache[imageCacheKey, []byte]
	size     atomic.Int64
	capacity int64
}

type imageCacheEntry struct {
	path string
	data []byte
}

func newImageCache(maxItems int, maxBytes int64) *ImageCache {
	cacheInstance := &ImageCache{
		capacity: maxBytes,
	}

	onEvict := func(key imageCacheKey, value []byte) {
		overhead := imageCacheEntryOverhead + len(key.path)
		cacheInstance.size.Add(-(int64(len(value)) + int64(overhead)))
	}

	cache, _ := lru.NewWithEvict[imageCacheKey, []byte](maxItems, onEvict)
	cacheInstance.cache = cache

	return cacheInstance
}

// Get retrieves a value from the cache.
func (cache *ImageCache) Get(key imageCacheKey) ([]byte, bool) {
	return cache.cache.Get(key)
}

// Set adds a value to the cache.
func (cache *ImageCache) Set(key imageCacheKey, data []byte) {
	overhead := imageCacheEntryOverhead + len(key.path)
	itemSize := len(data) + overhead

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
func (cache *ImageCache) Size() int64 { return cache.size.Load() }

var (
	globalImageCache     *ImageCache
	globalImageCacheOnce sync.Once
)

// GetImageCache returns the global in-memory image cache.
func GetImageCache() *ImageCache {
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

var imageCacheWriter struct {
	mu      sync.RWMutex
	channel chan imageCacheEntry
	once    sync.Once
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	logger  *slog.Logger
}

// InitImageCacheWriter initializes the background writer for the image cache.
func InitImageCacheWriter(ctx context.Context, logger *slog.Logger) {
	imageCacheWriter.once.Do(func() {
		if logger == nil {
			logger = slog.Default()
		}

		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan imageCacheEntry, imageCacheWriterBuffer)

		imageCacheWriter.mu.Lock()
		imageCacheWriter.logger = logger
		imageCacheWriter.cancel = cancel
		imageCacheWriter.channel = ch
		imageCacheWriter.mu.Unlock()

		imageCacheWriter.wg.Add(1)

		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    logger,
			Operation: "image cache writer",
			Fn: func() error {
				for {
					select {
					case cacheEntry, ok := <-ch:
						if !ok {
							return nil
						}
						_ = os.MkdirAll(filepath.Dir(cacheEntry.path), imageCacheDirMode)
						if err := os.WriteFile(cacheEntry.path, cacheEntry.data, imageCacheFileMode); err != nil {
							logger.Warn("Failed to write image cache file", "path", cacheEntry.path, "error", err)
						}
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			},
			Cleanup: imageCacheWriter.wg.Done,
		})
	})
}

// StopImageCacheWriter gracefully shuts down the background writer for the image cache.
func StopImageCacheWriter() {
	imageCacheWriter.mu.Lock()
	ch := imageCacheWriter.channel
	imageCacheWriter.channel = nil
	imageCacheWriter.mu.Unlock()

	if ch != nil {
		close(ch)
		imageCacheWriter.wg.Wait()
	}
}

func queueImageCacheWrite(path string, data []byte) {
	imageCacheWriter.mu.RLock()
	ch := imageCacheWriter.channel
	logger := imageCacheWriter.logger
	imageCacheWriter.mu.RUnlock()

	if ch == nil {
		InitImageCacheWriter(context.Background(), slog.Default())
		imageCacheWriter.mu.RLock()
		ch = imageCacheWriter.channel
		logger = imageCacheWriter.logger
		imageCacheWriter.mu.RUnlock()
	}

	if ch == nil {
		return
	}

	select {
	case ch <- imageCacheEntry{path: path, data: data}:
	default:
		if logger != nil {
			logger.Warn("Image cache writer channel full, dropping write", "path", path)
		}
	}
}

func shouldPreserveOriginal(name string) bool {
	_, ok := criticalOriginalPNGs[strings.ToLower(name)]
	return ok
}

func collectOriginalsToDelete(ctx context.Context, outputDir string, converted map[string]string) []string {
	keys := make([]string, 0, len(converted))
	for k := range converted {
		keys = append(keys, k)
	}

	toDelete := make([]string, 0, len(keys))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(ctx)
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

			absolutePath := fspkg.NormalizePath(filepath.Join(outputDir, originalPath))
			mu.Lock()
			toDelete = append(toDelete, absolutePath)
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return toDelete
}

func deleteOriginals(ctx context.Context, paths []string) int64 {
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

	errorGroup, _ := errgroup.WithContext(ctx)
	errorGroup.SetLimit(numWorkers)

	for _, fullPath := range paths {
		path := fullPath
		errorGroup.Go(func() error {
			if err := os.Remove(path); err == nil {
				deleted.Add(1)
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		})
	}
	_ = errorGroup.Wait()

	return deleted.Load()
}

// CleanupOriginalImages removes source image files (.png/.jpg/.jpeg) from the
// output directory when a corresponding .webp file exists.
func CleanupOriginalImages(ctx context.Context, outputDir string) {
	converted := GetConvertedImages()
	if len(converted) == 0 {
		return
	}
	toDelete := collectOriginalsToDelete(ctx, outputDir, converted)
	deleted := deleteOriginals(ctx, toDelete)
	if deleted > 0 {
		slog.Info("Cleaned up original images", "deleted", deleted)
	}
}

// ErrCacheMiss indicates the image was not found in the disk cache.
var ErrCacheMiss = errors.New("image not found in disk cache")

// CopyFromDiskCacheOptions configures a cache lookup and copy operation.
type CopyFromDiskCacheOptions struct {
	SrcFs        afero.Fs
	Sink         fspkg.ArtifactSink
	RelPath      string
	SrcPath      string
	DstPath      string
	CacheDir     string
	SrcInfo      os.FileInfo
	Metrics      ImageMetrics
	OnWrite      func(string)
	KeepOriginal bool
	MuteMetrics  bool
}

// CopyFromDiskCache attempts to copy an image from the persistent disk cache.
func CopyFromDiskCache(opts CopyFromDiskCacheOptions) error {
	if opts.CacheDir == "" {
		return ErrCacheMiss
	}

	key := imageCacheKey{
		path:    opts.SrcPath,
		size:    opts.SrcInfo.Size(),
		modTime: opts.SrcInfo.ModTime().UnixNano(),
	}

	hashStr := getImageHash(key)
	cacheFile := filepath.Join(opts.CacheDir, hashStr+".webp")

	if _, err := os.Stat(cacheFile); err != nil {
		return ErrCacheMiss
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return err
	}

	// Update memory cache
	GetImageCache().Set(key, data)

	if !opts.MuteMetrics && opts.Metrics != nil {
		opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(data)))
		opts.Metrics.IncrementAssetsProcessed()
	}

	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return err
	}

	if err := opts.Sink.WriteFile(opts.DstPath, data); err != nil {
		return err
	}

	if opts.OnWrite != nil {
		opts.OnWrite(opts.DstPath)
	}

	// Record conversion
	recordConvertedImage(opts.RelPath)

	if opts.KeepOriginal {
		_ = copyOriginalImage(opts)
	}

	return nil
}

func recordConvertedImage(relPath string) {
	relSource := "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	relDest := relSource[:len(relSource)-len(filepath.Ext(relSource))] + ".webp"
	RecordConvertedImage(relSource, relDest)
}

func copyOriginalImage(opts CopyFromDiskCacheOptions) error {
	ext := strings.ToLower(filepath.Ext(opts.SrcPath))
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
		originalDest := strings.TrimSuffix(opts.DstPath, ".webp") + ext
		return fspkg.CopyFileVFS(fspkg.CopyFileOptions{
			SrcFs:   opts.SrcFs,
			Sink:    opts.Sink,
			SrcPath: opts.SrcPath,
			DstPath: originalDest,
			ModTime: opts.SrcInfo.ModTime().UnixNano(),
			OnWrite: opts.OnWrite,
		})
	}
	return nil
}

