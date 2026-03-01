package utils

import (
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

	"github.com/spf13/afero"
	"github.com/twincats/golibvips/libvips"
	"github.com/zeebo/blake3"
)

type imageCache struct {
	mu   sync.RWMutex
	data map[string][]byte
	keys []string
	size int
	cap  int
}

func newImageCache(capacity int) *imageCache {
	return &imageCache{
		data: make(map[string][]byte),
		keys: make([]string, 0, 128),
		cap:  capacity,
	}
}

func (c *imageCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.data[key]
	return data, ok
}

func (c *imageCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.data[key]; ok {
		c.size -= len(existing)
		delete(c.data, key)
	}

	for c.size+len(data) > c.cap && len(c.keys) > 0 {
		oldKey := c.keys[0]
		c.keys = c.keys[1:]
		if oldData, ok := c.data[oldKey]; ok {
			c.size -= len(oldData)
			delete(c.data, oldKey)
		}
	}

	c.data[key] = data
	c.keys = append(c.keys, key)
	c.size += len(data)
}

var globalImageCache = newImageCache(50 * 1024 * 1024)

func CopyDirVFS(srcFs afero.Fs, destFs afero.Fs, srcDir, dstDir string, compress bool, excludeExts []string, onWrite func(string), cacheDir string, imageWorkers int) error {
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
	errChan := make(chan error, 100)
	var wg sync.WaitGroup

	numWorkers := imageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskQueue {
				ext := strings.ToLower(filepath.Ext(task.path))
				isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

				if compress && isImage {
					target := filepath.Join(dstDir, task.relPath)
					if err := processImageVFS(srcFs, destFs, task.path, target, cacheDir); err != nil {
						errChan <- fmt.Errorf("failed to process image %s: %w", task.path, err)
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
						errChan <- err
					}
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

		for _, exclude := range excludeExts {
			if ext == exclude {
				return nil
			}
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
	close(errChan)

	if err != nil {
		return err
	}

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

func processImageVFS(srcFs afero.Fs, destFs afero.Fs, srcPath, dstPath string, cacheDir string) error {
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

	img, err := libvips.NewImageFromFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to load image %s: %w", srcPath, err)
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
	webpParams.Quality = 80

	encodedData, _, err := img.ExportWebp(webpParams)
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", dstPath, err)
	}

	globalImageCache.set(memCacheKey, encodedData)

	if cacheFile != "" {
		if err := os.WriteFile(cacheFile, encodedData, 0644); err != nil {
			slog.Warn("Failed to write image cache file", "path", cacheFile, "error", err)
		}
	}

	return afero.WriteFile(destFs, dstPath, encodedData, 0644)
}
