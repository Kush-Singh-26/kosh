//go:build !wasm

package utils

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chai2010/webp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
	"golang.org/x/image/draw"
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
	size     atomic.Int64
	capacity int64
}

type fileTask struct {
	path    string
	relPath string
	info    fs.FileInfo
}

func newImageCache(maxItems int, maxBytes int64) *imageCache {
	ic := &imageCache{
		capacity: maxBytes,
	}

	onEvict := func(key imageCacheKey, value []byte) {
		// Calculate roughly same size as when it was added
		overhead := 128 + len(key.path) // struct overhead + string length
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
	// Calculate size with overhead
	overhead := 128 + len(key.path)
	itemSize := cap(data) + overhead

	c.size.Add(int64(itemSize))

	// Strict size-based eviction: remove oldest items until under capacity
	for c.size.Load() > c.capacity && c.cache.Len() > 0 {
		_, _, ok := c.cache.RemoveOldest()
		if !ok {
			break
		}
	}

	c.cache.Add(key, data)
}

// globalImageCache limits to 400 items or ~100MB of memory for better cache hit rates
var globalImageCache = newImageCache(400, 100*1024*1024)

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

var (
	copyBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}

	// rgbaPixPool reuses large byte slices for image resizing to reduce GC pressure
	rgbaPixPool = sync.Pool{
		New: func() any {
			// Pre-allocate for 1200px width * 1600px height * 4 bytes (RGBA)
			b := make([]byte, 1200*1600*4)
			return &b
		},
	}
)

// Async image cache writer
var imageCacheWriter struct {
	ch   chan imageCacheEntry
	once sync.Once
}

type imageCacheEntry struct {
	path string
	data []byte
}

func initImageCacheWriter() {
	imageCacheWriter.once.Do(func() {
		// Increased channel depth to smooth out I/O spikes
		imageCacheWriter.ch = make(chan imageCacheEntry, 256)
		// Launch multiple workers for async writes if needed, but 1 is usually enough for sequential disk
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
		// Make a copy since the caller's data may be from a sync.Pool
		dataCopy = make([]byte, len(data))
		copy(dataCopy, data)
	}

	select {
	case imageCacheWriter.ch <- imageCacheEntry{path: path, data: dataCopy}:
	default:
		// Channel full — write synchronously as fallback
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, dataCopy, 0644)
	}
}

const smallImageResizeThresholdBytes int64 = 32 * 1024

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

type ImageMetrics interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
}

type CopyOptions struct {
	Compress     bool
	ExcludeExts  []string
	OnWrite      func(string)
	CacheDir     string
	ImageWorkers int
	WebPQuality  int
	Metrics      ImageMetrics
}

func CopyFileVFS(srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, modTime int64, onWrite func(string)) error {
	if err := sink.MkdirAll(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(destPath), err)
	}

	// Attempt optimized syscall copy for local files
	if realSrc, ok := GetRealPath(srcFs, srcPath); ok {
		err := sink.CopyFile(realSrc, destPath)
		if err == nil {
			if onWrite != nil {
				onWrite(destPath)
			}
			if modTime > 0 {
				_ = sink.SetMtime(destPath, time.Unix(0, modTime))
			}
			return nil
		}
		// Fallback to streaming if optimized copy fails
		slog.Debug("Optimized copy failed, falling back to streaming", "src", srcPath, "error", err)
	}

	in, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer func() { _ = in.Close() }()

	bufPtr := copyBufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufferPool.Put(bufPtr)

	errWrite := sink.WriteStream(destPath, func(w io.Writer) error {
		_, err := io.CopyBuffer(w, in, buf)
		return err
	})
	if errWrite != nil {
		return fmt.Errorf("failed to copy file %s: %w", srcPath, errWrite)
	}

	if onWrite != nil {
		onWrite(destPath)
	}

	if modTime > 0 {
		_ = sink.SetMtime(destPath, time.Unix(0, modTime))
	}

	return nil
}

func CopyFileWithOptionalImageProcessing(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, info fs.FileInfo, opts CopyOptions) error {
	ext := strings.ToLower(filepath.Ext(srcPath))
	isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png"
	if opts.Compress && isImage {
		if err := processImageVFS(ctx, srcFs, sink, srcPath, destPath[:len(destPath)-len(filepath.Ext(destPath))]+".webp", srcInfoToFileInfo(info), opts); err != nil {
			return err
		}
		if opts.OnWrite != nil {
			opts.OnWrite(destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".webp")
		}
		return nil
	}
	modTime := int64(0)
	if info != nil {
		modTime = info.ModTime().UnixNano()
	}
	return CopyFileVFS(srcFs, sink, srcPath, destPath, modTime, opts.OnWrite)
}

// Helper to bridge FileInfo to srcInfo parameter in processImageVFS
func srcInfoToFileInfo(info fs.FileInfo) fs.FileInfo {
	return info
}

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcDir, dstDir string, opts CopyOptions) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	if err := sink.MkdirAll(dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	numWorkers := opts.ImageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	nonImageWorkers := max(numWorkers, 2)
	if nonImageWorkers > 32 {
		nonImageWorkers = 32
	}

	var errs []error
	var errMu sync.Mutex

	imageQueue := make(chan fileTask, 1024)
	nonImageQueue := make(chan fileTask, 1024)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-imageQueue:
					if !ok {
						return
					}
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("Image worker panic recovered", "panic", r)
								errMu.Lock()
								errs = append(errs, fmt.Errorf("image worker panicked on %s: %v", task.path, r))
								errMu.Unlock()
							}
						}()
						target := filepath.Join(dstDir, task.relPath)
						if err := processImageVFS(ctx, srcFs, sink, task.path, target, task.info, opts); err != nil {
							errMu.Lock()
							errs = append(errs, fmt.Errorf("failed to process image %s: %w", task.path, err))
							errMu.Unlock()
						} else if opts.OnWrite != nil {
							opts.OnWrite(target)
						}
					}()
				}
			}
		}()
	}
	for i := 0; i < nonImageWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-nonImageQueue:
					if !ok {
						return
					}
					destPath := filepath.Join(dstDir, task.relPath)
					if err := CopyFileVFS(srcFs, sink, task.path, destPath, task.info.ModTime().UnixNano(), opts.OnWrite); err != nil {
						errMu.Lock()
						errs = append(errs, err)
						errMu.Unlock()
					}
				}
			}
		}()
	}

	// Use higher concurrency for discovery walk on modern SSDs
	walkConcurrency := max(numWorkers/2, 4)
	walkErr := ParallelWalk(ctx, srcFs, srcDir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := SafeRel(srcDir, path)
		ext := strings.ToLower(filepath.Ext(path))
		baseName := filepath.Base(path)
		if baseName == "search.wasm" {
			return nil
		}
		isExcluded := false
		if baseName != "wasm_engine.js" && baseName != "engine.js" && baseName != "force-graph.js" && baseName != "wasm_exec.js" {
			if slices.Contains(opts.ExcludeExts, ext) {
				isExcluded = true
			}
		}
		if isExcluded {
			return nil
		}

		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")
		finalRelPath := relPath
		if opts.Compress && isImage {
			finalRelPath = relPath[:len(relPath)-len(ext)] + ".webp"
			select {
			case <-ctx.Done():
				return ctx.Err()
			case imageQueue <- fileTask{path, finalRelPath, info}:
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case nonImageQueue <- fileTask{path, finalRelPath, info}:
			}
		}
		return nil
	})

	close(imageQueue)
	close(nonImageQueue)
	wg.Wait()

	if walkErr != nil {
		return walkErr
	}
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func processImageVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcPath, dstPath string, srcInfo fs.FileInfo, opts CopyOptions) error {
	if srcInfo == nil {
		var err error
		srcInfo, err = srcFs.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("failed to stat source image %s: %w", srcPath, err)
		}
	}

	skipResize := srcInfo.Size() <= smallImageResizeThresholdBytes

	memCacheKey := imageCacheKey{
		path:    srcPath,
		size:    srcInfo.Size(),
		modTime: srcInfo.ModTime().UnixNano(),
	}

	if cached, ok := globalImageCache.get(memCacheKey); ok {
		if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
			return fmt.Errorf("failed to create image directory: %w", err)
		}
		if !isNil(opts.Metrics) {
			opts.Metrics.RecordImageOptimization(srcInfo.Size(), int64(len(cached)))
		}
		return sink.WriteFile(dstPath, cached)
	}

	var cacheFile string
	cacheFs := afero.NewOsFs()
	if opts.CacheDir != "" {
		hashStr := getImageHash(memCacheKey)
		cacheFile = filepath.Join(opts.CacheDir, hashStr+".webp")

		if cacheInfo, err := cacheFs.Stat(cacheFile); err == nil && !cacheInfo.IsDir() {
			// Stream cached file to sink instead of reading all into memory
			f, err := cacheFs.Open(cacheFile)
			if err != nil {
				return fmt.Errorf("failed to open cached image %s: %w", cacheFile, err)
			}
			defer func() { _ = f.Close() }()

			if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
				return fmt.Errorf("failed to create image directory: %w", err)
			}

			// Still set memory cache on hit
			cachedData, readErr := afero.ReadAll(f)
			if readErr == nil {
				globalImageCache.set(memCacheKey, cachedData)
				if !isNil(opts.Metrics) {
					opts.Metrics.RecordImageOptimization(srcInfo.Size(), int64(len(cachedData)))
				}
				if err := sink.WriteFile(dstPath, cachedData); err != nil {
					return fmt.Errorf("failed to write cached image %s: %w", dstPath, err)
				}
			} else {
				return fmt.Errorf("failed to read cached image %s: %w", cacheFile, readErr)
			}

			_ = sink.SetMtime(dstPath, srcInfo.ModTime())
			return nil
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Optimized: Acquire scheduler only for real work (cache miss)
	if err := GlobalScheduler.Acquire(ctx, TaskImage); err != nil {
		return err
	}
	defer GlobalScheduler.Release(TaskImage)

	// Direct stream from filesystem
	f, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", srcPath, err)
	}
	defer func() { _ = f.Close() }()

	// Use pooled buffered reader to reduce syscalls
	br := SharedBufioReaderPool.Get(f)
	defer func() {
		br.Reset(nil)
		SharedBufioReaderPool.Put(br)
	}()

	// Decode image
	src, _, err := image.Decode(br)
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %w", srcPath, err)
	}

	// Resize if needed
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	finalImg := src
	if width > 1200 && !skipResize {
		newWidth := 1200
		newHeight := (height * newWidth) / width

		// Use pooled pix buffer for resizing
		neededSize := newWidth * newHeight * 4
		var pix []byte
		var pixPtr *[]byte
		if neededSize <= 1200*1600*4 {
			pixPtr = rgbaPixPool.Get().(*[]byte)
			pix = *pixPtr
			defer rgbaPixPool.Put(pixPtr)
		} else {
			pix = make([]byte, neededSize)
		}

		dst := &image.RGBA{
			Pix:    pix[:neededSize],
			Stride: newWidth * 4,
			Rect:   image.Rect(0, 0, newWidth, newHeight),
		}

		draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		finalImg = dst
	} else if skipResize {
		if _, isYCbCr := finalImg.(*image.YCbCr); isYCbCr {
			b := finalImg.Bounds()
			rgba := image.NewRGBA(b)
			draw.Draw(rgba, b, finalImg, b.Min, draw.Src)
			finalImg = rgba
		}
		if !isNil(opts.Metrics) {
			opts.Metrics.RecordImageResizeSkipped()
		}
	}

	if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	webpQuality := opts.WebPQuality
	if webpQuality < 1 || webpQuality > 100 {
		webpQuality = 80
	}

	// Encode to WebP using pooled buffer
	buf := webpBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer webpBufferPool.Put(buf)

	err = webp.Encode(buf, finalImg, &webp.Options{Lossless: false, Quality: float32(webpQuality)})
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", dstPath, err)
	}
	encodedData := buf.Bytes()

	// Clone exact-sized slice for the LRU cache
	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	globalImageCache.set(memCacheKey, cacheData)

	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData, true)
	}

	if !isNil(opts.Metrics) {
		opts.Metrics.RecordImageOptimization(srcInfo.Size(), int64(len(cacheData)))
	}

	err = sink.WriteFile(dstPath, cacheData)
	if err == nil {
		_ = sink.SetMtime(dstPath, srcInfo.ModTime())
	}

	return err
}
