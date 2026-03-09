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
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
	"golang.org/x/image/draw"
)

type imageCache struct {
	cache    *lru.Cache[string, []byte]
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

	onEvict := func(key string, value []byte) {
		ic.mu.Lock()
		// Calculate roughly same size as when it was added
		overhead := 64 + len(key) // map entry overhead + string length
		ic.size -= (cap(value) + overhead)
		ic.mu.Unlock()
	}

	c, _ := lru.NewWithEvict[string, []byte](maxItems, onEvict)
	ic.cache = c

	return ic
}

func (c *imageCache) get(key string) ([]byte, bool) {
	return c.cache.Get(key)
}

func (c *imageCache) set(key string, data []byte) {
	// Calculate size with overhead
	overhead := 64 + len(key)
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

var copyBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 64*1024)
	},
}

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
		imageCacheWriter.ch = make(chan imageCacheEntry, 64)
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

func queueImageCacheWrite(path string, data []byte) {
	initImageCacheWriter()
	// Make a copy since the caller's data may be from a sync.Pool
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	select {
	case imageCacheWriter.ch <- imageCacheEntry{path: path, data: dataCopy}:
	default:
		// Channel full — write synchronously as fallback
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, data, 0644)
	}
}

var globalImageLimiter struct {
	mu    sync.RWMutex
	ch    chan struct{}
	limit int
}

const smallImageResizeThresholdBytes int64 = 12 * 1024

func SetGlobalImageProcessingLimit(limit int) {
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	if limit < 1 {
		limit = 1
	}

	globalImageLimiter.mu.Lock()
	defer globalImageLimiter.mu.Unlock()
	if globalImageLimiter.limit == limit && globalImageLimiter.ch != nil {
		return
	}
	globalImageLimiter.limit = limit
	globalImageLimiter.ch = make(chan struct{}, limit)
}

func withImageToken(ctx context.Context, fn func() error) error {
	globalImageLimiter.mu.RLock()
	ch := globalImageLimiter.ch
	globalImageLimiter.mu.RUnlock()
	if ch == nil {
		SetGlobalImageProcessingLimit(runtime.NumCPU())
		globalImageLimiter.mu.RLock()
		ch = globalImageLimiter.ch
		globalImageLimiter.mu.RUnlock()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- struct{}{}:
	}
	defer func() { <-ch }()

	return fn()
}

func isNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
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
		if err := withImageToken(ctx, func() error {
			return processImageVFS(ctx, srcFs, sink, srcPath, destPath[:len(destPath)-len(filepath.Ext(destPath))]+".webp", cacheDir, webpQuality, info, m)
		}); err != nil {
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
	nonImageWorkers := numWorkers
	if nonImageWorkers < 2 {
		nonImageWorkers = 2
	}
	if nonImageWorkers > 16 {
		nonImageWorkers = 16
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
						if err := withImageToken(ctx, func() error {
							return processImageVFS(ctx, srcFs, sink, task.path, target, cacheDir, webpQuality, task.info, m)
						}); err != nil {
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

	// Use low concurrency for discovery walk to avoid NTFS contention
	walkErr := ParallelWalk(ctx, srcFs, srcDir, 2, func(path string, info fs.FileInfo, err error) error {
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
			for _, exclude := range excludeExts {
				if ext == exclude {
					isExcluded = true
					break
				}
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

	memCacheKey := fmt.Sprintf("%s-%d-%d", srcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
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
		hash := xxh3.Hash128([]byte(memCacheKey))
		b := hash.Bytes()
		hashStr := hex.EncodeToString(b[:])
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

	// Direct stream from filesystem
	f, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", srcPath, err)
	}
	defer func() { _ = f.Close() }()

	// Decode image
	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %w", srcPath, err)
	}

	// Resize if needed
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	var finalImg image.Image = src
	if width > 1200 && !skipResize {
		newWidth := 1200
		newHeight := (height * newWidth) / width
		dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		finalImg = dst
	} else if skipResize && !isNil(m) {
		m.RecordImageResizeSkipped()
	}

	if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	if webpQuality < 1 || webpQuality > 100 {
		webpQuality = 80
	}

	// Encode to WebP
	var buf bytes.Buffer
	err = webp.Encode(&buf, finalImg, &webp.Options{Quality: float32(webpQuality)})
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", dstPath, err)
	}
	encodedData := buf.Bytes()

	// Clone exact-sized slice for the LRU cache
	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	globalImageCache.set(memCacheKey, cacheData)

	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData)
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
