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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/errgroup"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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

// ErrCacheMiss is returned when an image is not in the disk cache.
var ErrCacheMiss = errors.New("image not in disk cache")

// CopyFromDiskCache checks the on-disk WebP cache for an image and, if found,
// writes it to the sink and registers it in the converted-images map.
// Returns nil on cache hit, ErrCacheMiss on miss, or a real error.
// Called from the asset discovery goroutine to speed up image registration
// before background workers process the remaining cache-miss images.
func CopyFromDiskCache(srcFs afero.Fs, sink fspkg.ArtifactSink, relPath, srcPath, dstPath, cacheDir string, srcInfo fs.FileInfo, metrics ImageMetrics, onWrite func(string), keepOriginal bool, muteMetrics bool) error {
	cacheFs := afero.NewOsFs()
	memKey := imageCacheKey{
		path:    srcPath,
		size:    srcInfo.Size(),
		modTime: srcInfo.ModTime().UnixNano(),
	}
	hash := getImageHash(memKey)
	cacheFile := filepath.Join(cacheDir, hash+".webp")

	cachedData, err := afero.ReadFile(cacheFs, cacheFile)
	if err != nil {
		return ErrCacheMiss
	}

	if err := sink.WriteFile(dstPath, cachedData); err != nil {
		return err
	}

	if metrics != nil && !muteMetrics {
		metrics.RecordImageOptimization(srcInfo.Size(), int64(len(cachedData)))
		metrics.IncrementAssetsProcessed()
	}

	relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
	registerImageVariants(relSrc, relDst)

	if onWrite != nil {
		onWrite(dstPath)
	}

	// If keepOriginal is requested, also copy the source file to its destination
	if keepOriginal {
		ext := strings.ToLower(filepath.Ext(srcPath))
		origDst := strings.TrimSuffix(dstPath, filepath.Ext(dstPath)) + ext
		_ = fspkg.CopyFileVFS(fspkg.CopyFileOptions{
			SrcFs:   srcFs,
			Sink:    sink,
			SrcPath: srcPath,
			DstPath: origDst,
			ModTime: srcInfo.ModTime().UnixNano(),
			OnWrite: onWrite,
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

// RewriteImagePaths rewrites image paths in output HTML files from original
// extensions (.png/.jpg/.jpeg) to .webp for images processed in the background.
// Called after all images have been processed and all HTML files have been written.
// outputDir must point to the actual output directory (staging dir for clean builds).
//
// Two-pass approach:
// Pass 1: Use the conversion map (exact path matching) for images known to the current build.
// Pass 2: Scan the output directory for .webp files and rewrite any remaining references.
// This ensures cached HTML (from previous builds) is also correctly rewritten.
//
// Optimization: If no images were converted in this build (empty converted map),
// we do a quick check to see if any webp files exist. If not, we skip entirely.
func RewriteImagePaths(outputDir string) {
	converted := GetConvertedImages()

	// Quick optimization: if no images were converted, check if any webp files exist.
	// If not, skip the expensive full walk.
	if len(converted) == 0 {
		hasWebP := false
		_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), ".webp") {
				hasWebP = true
				return fs.SkipAll
			}
			return nil
		})
		if !hasWebP {
			return
		}
	}

	// Pass 1: Build rewrite map from registered conversion variants
	type rewriteEntry struct{ from, to string }
	var rewrites []rewriteEntry
	for orig, webp := range converted {
		lower := strings.ToLower(orig)
		if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") {
			rewrites = append(rewrites, rewriteEntry{from: orig, to: webp})
		}
	}

	// Longest-first prevents partial matches: "Transformer10.png" won't be
	// matched by a rule for "Transformer1.png".
	sort.Slice(rewrites, func(i, j int) bool {
		return len(rewrites[i].from) > len(rewrites[j].from)
	})

	// Pass 2: Scan output directory for .webp files and build filename-only mapping.
	// This catches HTML from cached posts that still reference original extensions.
	// Maps basename-without-ext (lowercase) → webp basename.
	// e.g. "transformer1" → "Transformer1.webp"
	webpFiles := make(map[string]string)
	_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".webp") {
			return nil
		}
		base := filepath.Base(path)
		baseNoExt := base[:len(base)-len(filepath.Ext(base))]
		key := strings.ToLower(baseNoExt)
		webpFiles[key] = base
		return nil
	})

	// Collect HTML file paths
	var htmlPaths []string
	_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
			htmlPaths = append(htmlPaths, path)
		}
		return nil
	})

	// Process HTML files in parallel
	var totalRewrites atomic.Int64
	numWorkers := runtime.NumCPU()
	if numWorkers > len(htmlPaths) {
		numWorkers = len(htmlPaths)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(numWorkers)

	for _, path := range htmlPaths {
		p := path
		g.Go(func() error {
			data, readErr := os.ReadFile(filepath.Join(outputDir, p))
			if readErr != nil {
				return nil
			}

			modified := false
			fileRewrites := 0

			// Pass 1: Exact path rewrites from conversion map
			for _, r := range rewrites {
				if bytes.Contains(data, []byte(r.from)) {
					data = bytes.ReplaceAll(data, []byte(r.from), []byte(r.to))
					modified = true
					fileRewrites++
				}
			}

			// Pass 2: Basename-level rewrites using webp files on disk.
			// Fast path: skip if file contains no known image extensions
			hasImageExt := false
			for _, ext := range []string{".png", ".jpg", ".jpeg"} {
				if bytes.Contains(data, []byte(ext)) {
					hasImageExt = true
					break
				}
			}

			if hasImageExt {
				dataStr := string(data)
				dataLower := strings.ToLower(dataStr)
				for baseNoExtLower := range webpFiles {
					for _, ext := range []string{".png", ".jpg", ".jpeg"} {
						searchTarget := baseNoExtLower + ext
						idx := 0
						for {
							foundIdx := strings.Index(dataLower[idx:], searchTarget)
							if foundIdx < 0 {
								break
							}
							absIdx := idx + foundIdx
							idx = absIdx + len(searchTarget)

							// Extract the original filename (preserving case)
							end := absIdx + len(searchTarget)
							original := dataStr[absIdx:end]
							// Only replace the extension part: keep prefix, swap ext
							originalBaseNoExt := original[:len(original)-len(ext)]
							replacement := originalBaseNoExt + ".webp"
							dataStr = dataStr[:absIdx] + replacement + dataStr[end:]
							dataLower = strings.ToLower(dataStr)
							modified = true
							fileRewrites++
							// Adjust idx because replacement length changed
							idx += len(replacement) - len(original)
						}
					}
				}
				if modified && fileRewrites > 0 {
					data = []byte(dataStr)
				}
			}

			if modified {
				totalRewrites.Add(int64(fileRewrites))
				_ = os.WriteFile(filepath.Join(outputDir, p), data, 0644)
			}
			return nil
		})
	}
	_ = g.Wait()
}

// CleanupOriginalImages removes source image files (.png/.jpg/.jpeg) from the
// output directory when a corresponding .webp file exists. This ensures only
// WebP images remain in the built output.
//
// Critical assets preserved as .png:
//   - favicon.png (required by browsers)
//   - icon-192.png, icon-512.png (required by PWA manifest)
func CleanupOriginalImages(outputDir string) {
	// Quick check: if no webp files exist, skip entirely
	hasWebP := false
	_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".webp") {
			hasWebP = true
			return fs.SkipAll
		}
		return nil
	})
	if !hasWebP {
		return
	}

	// Collect relative paths (without extension) of all .webp files
	webpRels := make(map[string]bool)
	_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".webp") {
			return nil
		}
		relNoExt := strings.TrimSuffix(path, filepath.Ext(path))
		webpRels[strings.ToLower(relNoExt)] = true
		return nil
	})

	// Critical .png files that must be preserved
	criticalPNGs := map[string]bool{
		"favicon.png":  true,
		"icon-192.png": true,
		"icon-512.png": true,
	}

	// Collect paths to delete (sequential walk, fast since no I/O)
	var toDelete []string
	_ = fs.WalkDir(os.DirFS(outputDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") {
			return nil
		}
		base := filepath.Base(path)
		if criticalPNGs[strings.ToLower(base)] {
			return nil
		}
		relNoExt := strings.TrimSuffix(path, filepath.Ext(path))
		if webpRels[strings.ToLower(relNoExt)] {
			toDelete = append(toDelete, filepath.Join(outputDir, path))
		}
		return nil
	})

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
			}
			return nil
		})
	}
	_ = g.Wait()

	if d := deleted.Load(); d > 0 {
		slog.Info("Cleaned up original images", "deleted", d)
	}
}
