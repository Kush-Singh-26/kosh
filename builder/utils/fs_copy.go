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
	"runtime"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/twincats/golibvips/libvips"
	"github.com/zeebo/blake3"
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

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, destFs afero.Fs, srcDir, dstDir string, compress bool, excludeExts []string, onWrite func(string), cacheDir string, imageWorkers int, webpQuality int) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	if err := destFs.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	type fileTask struct {
		path    string
		relPath string
		info    fs.FileInfo
	}

	taskQueue := make(chan fileTask, 100)
	var errs []error
	var errMu sync.Mutex
	var wg sync.WaitGroup

	numWorkers := imageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
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
							if err := processImageVFS(ctx, srcFs, destFs, task.path, target, cacheDir, webpQuality); err != nil {
								errMu.Lock()
								errs = append(errs, fmt.Errorf("failed to process image %s: %w", task.path, err))
								errMu.Unlock()
							} else if onWrite != nil {
								onWrite(target)
							}
						} else {
							destPath := filepath.Join(dstDir, task.relPath)
							err := func() error {
								destDir := filepath.Dir(destPath)
								if err := destFs.MkdirAll(destDir, 0755); err != nil {
									return fmt.Errorf("failed to create directory %s: %w", destDir, err)
								}

								in, err := srcFs.Open(task.path)
								if err != nil {
									return fmt.Errorf("failed to open source file %s: %w", task.path, err)
								}
								defer func() { _ = in.Close() }()

								out, err := destFs.Create(destPath)
								if err != nil {
									return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
								}
								defer func() { _ = out.Close() }()

								if _, err := io.Copy(out, in); err != nil {
									return fmt.Errorf("failed to copy file %s: %w", task.path, err)
								}
								if onWrite != nil {
									onWrite(destPath)
								}
								return nil
							}()
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

	err := afero.Walk(srcFs, srcDir, func(path string, info fs.FileInfo, err error) error {
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
		if baseName != "wasm_engine.js" && baseName != "engine.js" && baseName != "force-graph.js" {
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

		taskQueue <- fileTask{path, finalRelPath, info}
		return nil
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

func processImageVFS(ctx context.Context, srcFs afero.Fs, destFs afero.Fs, srcPath, dstPath string, cacheDir string, webpQuality int) error {
	srcInfo, err := srcFs.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("failed to stat source image %s: %w", srcPath, err)
	}

	memCacheKey := fmt.Sprintf("%s-%d-%d", srcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
	if cached, ok := globalImageCache.get(memCacheKey); ok {
		if err := destFs.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create image directory: %w", err)
		}
		return afero.WriteFile(destFs, dstPath, cached, 0644)
	}

	if dstInfo, err := os.Stat(dstPath); err == nil {
		if !srcInfo.ModTime().After(dstInfo.ModTime()) {
			data, err := os.ReadFile(dstPath)
			if err == nil {
				globalImageCache.set(memCacheKey, data)
				return afero.WriteFile(destFs, dstPath, data, 0644)
			}
		}
	}

	var cacheFile string
	if cacheDir != "" {
		hash := blake3.Sum256([]byte(memCacheKey))
		hashStr := hex.EncodeToString(hash[:])
		cacheFile = filepath.Join(cacheDir, hashStr+".webp")

		if data, err := os.ReadFile(cacheFile); err == nil {
			globalImageCache.set(memCacheKey, data)
			if err := destFs.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return fmt.Errorf("failed to create image directory: %w", err)
			}
			return WriteFileVFS(destFs, dstPath, data)
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

	if img.Width() > 1200 {
		if err := img.ResizeWidthPixel(1200, libvips.KernelAuto); err != nil {
			return fmt.Errorf("failed to resize image %s: %w", srcPath, err)
		}
	}

	if err := destFs.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
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
		if err := os.WriteFile(cacheFile, finalData, 0644); err != nil {
			slog.Warn("Failed to write image cache file", "path", cacheFile, "error", err)
		}
	}

	err = afero.WriteFile(destFs, dstPath, finalData, 0644)

	return err
}
