//go:build !wasm

package utils

import (
	"context"
	"encoding/hex"
	"fmt"
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
	"time"

	"github.com/h2non/bimg"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

type imageCacheKey struct {
	path    string
	size    int64
	modTime int64
}

type imageCache struct {
	cache    *lru.Cache[imageCacheKey, []byte]
	mu       sync.RWMutex
	size     int
	capacity int
}

type fileTask struct {
	path    string
	relPath string
	info    fs.FileInfo
}

func newImageCache(maxItems int, maxBytes int) *imageCache {
	ic := &imageCache{
		capacity: maxBytes,
	}

	onEvict := func(key imageCacheKey, value []byte) {
		ic.mu.Lock()
		// Calculate roughly same size as when it was added
		overhead := 128 + len(key.path) // struct overhead + string length
		ic.size -= (cap(value) + overhead)
		ic.mu.Unlock()
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

	c.mu.Lock()
	c.size += itemSize

	// Strict size-based eviction: remove oldest items until under capacity
	for c.size > c.capacity && c.cache.Len() > 0 {
		_, _, ok := c.cache.RemoveOldest()
		if !ok {
			break
		}
	}
	c.mu.Unlock()

	c.cache.Add(key, data)
}

// globalImageCache limits to 200 items or ~50MB of memory
var globalImageCache = newImageCache(200, 50*1024*1024)

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
			return make([]byte, 64*1024)
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

func CopyFileVFS(srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, modTime int64, onWrite func(string)) error {
	if err := sink.MkdirAll(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(destPath), err)
	}

	in, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer func() { _ = in.Close() }()

	buf := copyBufferPool.Get().([]byte)
	defer copyBufferPool.Put(buf)

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

func copyFileBetweenFS(srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, modTime int64, onWrite func(string)) error {
	return CopyFileVFS(srcFs, sink, srcPath, destPath, modTime, onWrite)
}

func CopyFileWithOptionalImageProcessing(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, compress bool, cacheDir string, webpQuality int, info fs.FileInfo, m interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
}, onWrite func(string)) error {
	ext := strings.ToLower(filepath.Ext(srcPath))
	isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png"
	if compress && isImage {
		if err := GlobalScheduler.Acquire(ctx, TaskImage); err != nil {
			return err
		}
		defer GlobalScheduler.Release(TaskImage)

		if err := processImageVFS(ctx, srcFs, sink, srcPath, destPath[:len(destPath)-len(filepath.Ext(destPath))]+".webp", cacheDir, webpQuality, info, m); err != nil {
			return err
		}
		if onWrite != nil {
			onWrite(destPath[:len(destPath)-len(filepath.Ext(destPath))] + ".webp")
		}
		return nil
	}
	modTime := int64(0)
	if info != nil {
		modTime = info.ModTime().UnixNano()
	}
	return CopyFileVFS(srcFs, sink, srcPath, destPath, modTime, onWrite)
}

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcDir, dstDir string, compress bool, excludeExts []string, onWrite func(string), cacheDir string, imageWorkers int, webpQuality int, m interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
}) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	if err := sink.MkdirAll(dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	numWorkers := imageWorkers
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
							}
						}()
						target := filepath.Join(dstDir, task.relPath)
						if err := GlobalScheduler.Acquire(ctx, TaskImage); err != nil {
							errMu.Lock()
							errs = append(errs, err)
							errMu.Unlock()
							return
						}
						defer GlobalScheduler.Release(TaskImage)

						if err := processImageVFS(ctx, srcFs, sink, task.path, target, cacheDir, webpQuality, task.info, m); err != nil {
							errMu.Lock()
							errs = append(errs, fmt.Errorf("failed to process image %s: %w", task.path, err))
							errMu.Unlock()
						} else if onWrite != nil {
							onWrite(target)
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
					if err := CopyFileVFS(srcFs, sink, task.path, destPath, task.info.ModTime().UnixNano(), onWrite); err != nil {
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
			if slices.Contains(excludeExts, ext) {
				isExcluded = true
			}
		}
		if isExcluded {
			return nil
		}

		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")
		finalRelPath := relPath
		if compress && isImage {
			finalRelPath = relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".webp"
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

func processImageVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcPath, dstPath string, cacheDir string, webpQuality int, srcInfo fs.FileInfo, m interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
}) error {
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
		if !isNil(m) {
			m.RecordImageOptimization(srcInfo.Size(), int64(len(cached)))
		}
		return sink.WriteFile(dstPath, cached)
	}

	var cacheFile string
	cacheFs := afero.NewOsFs()
	if cacheDir != "" {
		hashStr := getImageHash(memCacheKey)
		cacheFile = filepath.Join(cacheDir, hashStr+".webp")

		if cacheInfo, err := cacheFs.Stat(cacheFile); err == nil && !cacheInfo.IsDir() {
			cachedData, readErr := afero.ReadFile(cacheFs, cacheFile)
			if readErr != nil {
				return fmt.Errorf("failed to read cached image %s: %w", cacheFile, readErr)
			}
			globalImageCache.set(memCacheKey, cachedData)

			if !isNil(m) {
				m.RecordImageOptimization(srcInfo.Size(), int64(len(cachedData)))
			}
			if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
				return fmt.Errorf("failed to create image directory: %w", err)
			}
			if err := sink.WriteFile(dstPath, cachedData); err != nil {
				return fmt.Errorf("failed to write cached image %s: %w", dstPath, err)
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

	// Read the entire source file into memory
	imgData, err := afero.ReadFile(srcFs, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read image %s: %w", srcPath, err)
	}

	// Initialize bimg
	img := bimg.NewImage(imgData)
	size, err := img.Size()
	if err != nil {
		return fmt.Errorf("failed to get image size: %w", err)
	}

	if webpQuality < 1 || webpQuality > 100 {
		webpQuality = 80
	}

	// Setup processing options
	opts := bimg.Options{
		Type:    bimg.WEBP,
		Quality: webpQuality,
	}

	// Handle resizing
	if size.Width > 1200 && !skipResize {
		opts.Width = 1200
		opts.Height = (size.Height * 1200) / size.Width
	} else if skipResize && !isNil(m) {
		m.RecordImageResizeSkipped()
	}

	if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	// Process the image (C-level execution of decode, resize, encode)
	encodedData, err := img.Process(opts)
	if err != nil {
		return fmt.Errorf("failed to process image with libvips: %w", err)
	}

	// Clone exact-sized slice for the LRU cache
	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	globalImageCache.set(memCacheKey, cacheData)

	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData, true)
	}

	if !isNil(m) {
		m.RecordImageOptimization(srcInfo.Size(), int64(len(cacheData)))
	}

	err = sink.WriteFile(dstPath, cacheData)
	if err == nil {
		_ = sink.SetMtime(dstPath, srcInfo.ModTime())
	}

	return err
}
