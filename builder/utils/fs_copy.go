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
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/twincats/golibvips/libvips"
	"github.com/zeebo/xxh3"
)

type imageCache struct {
	cache    *lru.Cache[string, []byte]
	mu       sync.RWMutex
	size     int
	capacity int
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
	// We use a safe loop to prevent infinite eviction if a single item > capacity
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

func copyFileVFS(srcFs afero.Fs, sink ArtifactSink, srcPath, destPath string, modTime int64, onWrite func(string)) error {
	if err := sink.MkdirAll(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(destPath), err)
	}

	in, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer func() { _ = in.Close() }()

	errWrite := sink.WriteStream(destPath, func(w io.Writer) error {
		_, err := io.Copy(w, in)
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

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcDir, dstDir string, compress bool, excludeExts []string, onWrite func(string), cacheDir string, imageWorkers int, webpQuality int, m interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
}) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	if err := sink.MkdirAll(dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	type fileTask struct {
		path    string
		relPath string
		info    fs.FileInfo
	}

	numWorkers := imageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}

	taskQueue := make(chan fileTask, numWorkers*4)
	var errs []error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer libvips.ShutdownThread() // Ensure C-side thread-local caches are released
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-taskQueue:
					if !ok {
						return
					}
					// Recover from panics to prevent worker crashes
					func() {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("Image worker panic recovered", "panic", r)
							}
						}()
						ext := strings.ToLower(filepath.Ext(task.path))
						isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

						if compress && isImage {
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
						} else {
							destPath := filepath.Join(dstDir, task.relPath)
							err := copyFileVFS(srcFs, sink, task.path, destPath, task.info.ModTime().UnixNano(), onWrite)
							if err != nil {
								errMu.Lock()
								errs = append(errs, err)
								errMu.Unlock()
							}
						}
					}() // Close panic recovery wrapper
				}
			}
		}()
	}

	err := afero.Afero{Fs: srcFs}.Walk(filepath.FromSlash(srcDir), func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := SafeRel(srcDir, path)
		ext := strings.ToLower(filepath.Ext(path))
		baseName := filepath.Base(path)
		isExcluded := false
		if baseName == "search.wasm" {
			return nil
		}
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
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case taskQueue <- fileTask{path, finalRelPath, info}:
			return nil
		}
	})

	close(taskQueue)
	wg.Wait()

	if err != nil {
		return err
	}

	if len(errs) > 0 {
		return errs[0] // Return first error
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

	// We cannot safely read from Sink to check dstInfo. 
	// We will just process if it's not in cache.

	var cacheFile string
	cacheFs := afero.NewOsFs()
	if cacheDir != "" {
		hash := xxh3.Hash128([]byte(memCacheKey))
		b := hash.Bytes()
		hashStr := hex.EncodeToString(b[:])
		cacheFile = filepath.Join(cacheDir, hashStr+".webp")

		if data, err := afero.ReadFile(cacheFs, cacheFile); err == nil {
			globalImageCache.set(memCacheKey, data)
			if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
				return fmt.Errorf("failed to create image directory: %w", err)
			}
			if !isNil(m) {
				m.RecordImageOptimization(srcInfo.Size(), int64(len(data)))
			}
			return sink.WriteFile(dstPath, data)
		}
	}

	// Check context before heavy image operation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	imgData, err := afero.ReadFile(srcFs, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read image %s from srcFs: %w", srcPath, err)
	}

	img, err := libvips.NewImageFromBuffer(imgData)
	if err != nil {
		return fmt.Errorf("failed to parse image buffer %s: %w", srcPath, err)
	}
	defer img.Close()

	if img.Width() > 1200 && !skipResize {
		if err := img.ResizeWidthPixel(1200, libvips.KernelAuto); err != nil {
			return fmt.Errorf("failed to resize image %s: %w", srcPath, err)
		}
	} else if skipResize && !isNil(m) {
		m.RecordImageResizeSkipped()
	}

	if err := sink.MkdirAll(filepath.Dir(dstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	webpParams := libvips.NewWebpExportParams()
	if webpQuality < 1 || webpQuality > 100 {
		webpQuality = 80 // fallback to default
	}
	webpParams.Quality = webpQuality

	encodedData, _, err := img.ExportWebp(webpParams)
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", dstPath, err)
	}

	// Copy to pooled slice before libvips frees the memory
	pooledBytes := SharedByteSlicePool.Get()
	defer SharedByteSlicePool.Put(pooledBytes)

	if cap(*pooledBytes) < len(encodedData) {
		*pooledBytes = make([]byte, len(encodedData))
	} else {
		*pooledBytes = (*pooledBytes)[:len(encodedData)]
	}
	copy(*pooledBytes, encodedData)
	finalData := *pooledBytes

	// Clone exact-sized slice for the LRU cache so we don't pin the massive pooled slice array in RAM
	cacheData := make([]byte, len(finalData))
	copy(cacheData, finalData)
	globalImageCache.set(memCacheKey, cacheData)

	if cacheFile != "" {
		_ = os.MkdirAll(filepath.Dir(cacheFile), 0755)
		if err := afero.WriteFile(cacheFs, cacheFile, finalData, 0644); err != nil {
			slog.Warn("Failed to write image cache file", "path", cacheFile, "error", err)
		}
	}

	if !isNil(m) {
		m.RecordImageOptimization(srcInfo.Size(), int64(len(finalData)))
	}

	err = sink.WriteFile(dstPath, finalData)
	if err == nil {
		_ = sink.SetMtime(dstPath, srcInfo.ModTime())
	}

	return err
}
