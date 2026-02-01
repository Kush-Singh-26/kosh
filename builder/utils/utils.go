// Helper functions for file copying, image processing, and data sorting
package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"my-ssg/builder/config"
	"my-ssg/builder/models"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

// Global Minifier Instance
var Minifier *minify.M

func InitMinifier() {
	Minifier = minify.New()
	Minifier.AddFunc("text/html", html.Minify)
	Minifier.AddFunc("text/css", css.Minify)
	Minifier.AddFunc("text/javascript", js.Minify)
}

// ProcessAssets handles minification and fingerprinting of CSS and JS files.
// It returns a map of original filenames to their hashed counterparts.
// AssetTask represents a CSS/JS file to process
type AssetTask struct {
	srcPath string
	destDir string
	srcDir  string
}

// AssetResult contains the processed asset info
type AssetResult struct {
	key   string
	value string
	err   error
}

func ProcessAssets(srcDir, destDir string) (map[string]string, error) {
	assets := make(map[string]string)
	tasks := make(chan AssetTask, 50)
	results := make(chan AssetResult, 50)
	var wg sync.WaitGroup

	// Start 8 workers for parallel processing
	numWorkers := 8
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				key, val, err := processAssetFile(task.srcPath, task.destDir, task.srcDir)
				results <- AssetResult{key, val, err}
			}
		}()
	}

	// Close results channel when all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Walk directory and queue tasks
	var walkErr error
	go func() {
		defer close(tasks)
		walkErr = filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".css" && ext != ".js" {
				return nil
			}

			tasks <- AssetTask{srcPath: path, destDir: destDir, srcDir: srcDir}
			return nil
		})
	}()

	// Collect results
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		assets[result.key] = result.value
	}

	if walkErr != nil {
		return nil, walkErr
	}

	return assets, nil
}

func processAssetFile(path, destDir, srcDir string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(path))

	// Get file info for caching
	info, err := os.Stat(path)
	if err != nil {
		return "", "", err
	}

	// Construct new filename base
	relPath, _ := filepath.Rel(srcDir, path)
	dir := filepath.Dir(relPath)
	filename := strings.TrimSuffix(filepath.Base(path), ext)

	// Read original content
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	// Minify
	var minifiedContent []byte
	var mediaType string
	if ext == ".css" {
		mediaType = "text/css"
	} else {
		mediaType = "text/javascript"
	}

	// Use minifier if initialized, otherwise use raw
	if Minifier != nil {
		b := &strings.Builder{}
		if err := Minifier.Minify(mediaType, b, strings.NewReader(string(content))); err == nil {
			minifiedContent = []byte(b.String())
		} else {
			// Fallback to original if minification fails
			fmt.Printf("⚠️ Minification failed for %s: %v\n", path, err)
			minifiedContent = content
		}
	} else {
		minifiedContent = content
	}

	// Generate Hash (skip in dev mode for CSS/JS to enable hot-reload)
	var hashedFilename string
	var shortHash string
	if config.IsDevMode() && (ext == ".css" || ext == ".js") {
		// In dev mode, use unhashed filenames for CSS/JS
		// This allows browsers to cache the file and update on edit
		hashedFilename = fmt.Sprintf("%s%s", filename, ext)
	} else {
		// In production, use hashed filenames for cache busting
		hash := sha256.Sum256(minifiedContent)
		shortHash = hex.EncodeToString(hash[:])[:8]
		hashedFilename = fmt.Sprintf("%s.%s%s", filename, shortHash, ext)
	}

	// Map creation: normalized keys (e.g., /static/css/theme.css)
	relKeyPath := filepath.Join("/", "static", relPath)
	key := filepath.ToSlash(relKeyPath)
	val := filepath.ToSlash(filepath.Join("/static", dir, hashedFilename))

	// Check if destination already exists and is up-to-date
	destFile := filepath.Join(destDir, dir, hashedFilename)
	if destInfo, err := os.Stat(destFile); err == nil {
		// File exists - check if source is older (means we can skip writing)
		if info.ModTime().Before(destInfo.ModTime()) || info.ModTime().Equal(destInfo.ModTime()) {
			// Source hasn't changed, destination exists with correct hash
			return key, val, nil
		}
	}

	// Write to destination
	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return "", "", err
	}

	if err := os.WriteFile(destFile, minifiedContent, 0644); err != nil {
		return "", "", err
	}

	return key, val, nil
}

// CopyDir copies a directory recursively with parallel directory walking and async image processing.
// This version walks directories in parallel using 8 workers and processes images immediately as they're discovered.
func CopyDir(src, dst string, compress bool) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", src)
	}

	_ = os.MkdirAll(dst, 0755)

	// Channels for parallel processing
	type fileTask struct {
		srcPath  string
		dstPath  string
		isImage  bool
		fileInfo os.FileInfo
	}

	dirQueue := make(chan string, 100)    // Directories to walk
	fileQueue := make(chan fileTask, 100) // Files to process (images and non-images)
	imageQueue := make(chan fileTask, 50) // Images for parallel conversion
	errChan := make(chan error, 100)

	var wg sync.WaitGroup
	var dirWg sync.WaitGroup

	// Count images and processed images for progress
	var totalImages int32
	var processedImages int32
	var mu sync.Mutex

	// Track when all images are done processing so we can close imageQueue early
	var imagesDone int32
	var imageQueueCloseOnce sync.Once
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			total := atomic.LoadInt32(&totalImages)
			processed := atomic.LoadInt32(&processedImages)
			if total > 0 && total == processed {
				// All images found and processed, mark as done and close imageQueue
				atomic.StoreInt32(&imagesDone, 1)
				imageQueueCloseOnce.Do(func() {
					close(imageQueue)
				})
				return
			}
		}
	}()

	// Start directory walkers (parallel) - 8 workers max to avoid filesystem contention
	numDirWorkers := runtime.NumCPU()
	if numDirWorkers > 8 {
		numDirWorkers = 8
	}

	for i := 0; i < numDirWorkers; i++ {
		go func() {
			for dir := range dirQueue {
				entries, err := os.ReadDir(dir)
				if err != nil {
					errChan <- err
					dirWg.Done()
					continue
				}

				for _, entry := range entries {
					srcPath := filepath.Join(dir, entry.Name())
					relPath, _ := filepath.Rel(src, srcPath)
					dstPath := filepath.Join(dst, relPath)

					if entry.IsDir() {
						// Create directory and queue for walking
						if err := os.MkdirAll(dstPath, 0755); err != nil {
							errChan <- err
							continue
						}
						dirWg.Add(1)
						select {
						case dirQueue <- srcPath:
						default:
							// Queue full, process synchronously
							entries2, _ := os.ReadDir(srcPath)
							for _, e2 := range entries2 {
								srcPath2 := filepath.Join(srcPath, e2.Name())
								relPath2, _ := filepath.Rel(src, srcPath2)
								dstPath2 := filepath.Join(dst, relPath2)
								if e2.IsDir() {
									os.MkdirAll(dstPath2, 0755)
									dirWg.Add(1)
									dirQueue <- srcPath2
								} else {
									info, _ := e2.Info()
									fileQueue <- fileTask{srcPath2, dstPath2, false, info}
								}
							}
							dirWg.Done()
						}
					} else {
						// Check file type
						info, err := entry.Info()
						if err != nil {
							continue
						}

						ext := strings.ToLower(filepath.Ext(srcPath))
						isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

						if compress && isImage {
							extLen := len(filepath.Ext(dstPath))
							dstPath = dstPath[:len(dstPath)-extLen] + ".webp"
						}

						// Check if needs processing (incremental build)
						if destInfo, err := os.Stat(dstPath); err == nil {
							if destInfo.ModTime().After(info.ModTime()) {
								continue // Skip up-to-date files
							}
						}

						// Queue for processing
						if compress && isImage {
							// Only queue if images aren't already done processing
							if atomic.LoadInt32(&imagesDone) == 0 {
								atomic.AddInt32(&totalImages, 1)
								imageQueue <- fileTask{srcPath, dstPath, true, info}
							}
						} else {
							fileQueue <- fileTask{srcPath, dstPath, false, info}
						}
					}
				}
				dirWg.Done()
			}
		}()
	}

	// Start file processors (non-image files) - 8 workers for parallel processing
	numFileWorkers := 8
	var fileWg sync.WaitGroup

	for i := 0; i < numFileWorkers; i++ {
		fileWg.Add(1)
		go func() {
			defer fileWg.Done()
			for task := range fileQueue {
				srcPath := task.srcPath
				dstPath := task.dstPath
				ext := strings.ToLower(filepath.Ext(srcPath))

				if compress {
					if ext == ".css" {
						if err := minifyFile("text/css", srcPath, dstPath); err != nil {
							errChan <- err
						}
						continue
					}
					if ext == ".js" {
						if err := minifyFile("text/javascript", srcPath, dstPath); err != nil {
							errChan <- err
						}
						continue
					}
				}

				// Standard copy for other files
				if err := CopyFileStandard(srcPath, dstPath); err != nil {
					errChan <- err
				}
			}
		}()
	}

	// Start image processors (24 workers)
	numImageWorkers := 24
	fmt.Printf("🖼️  Starting parallel image processing with %d workers...\n", numImageWorkers)

	for i := 0; i < numImageWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range imageQueue {
				if err := processImage(task.srcPath, task.dstPath); err != nil {
					errChan <- fmt.Errorf("failed to process %s: %w", task.srcPath, err)
				}

				count := atomic.AddInt32(&processedImages, 1)
				total := atomic.LoadInt32(&totalImages)
				if count%5 == 0 || count == total {
					mu.Lock()
					fmt.Printf("   📊 Images: %d/%d converted\n", count, total)
					mu.Unlock()
				}
			}
		}()
	}

	// Start the walk with immediate feedback
	fmt.Printf("📂 Scanning static directory with %d parallel walkers...\n", numDirWorkers)
	dirWg.Add(1)
	dirQueue <- src

	// Wait for directory walking to complete
	go func() {
		dirWg.Wait()
		fmt.Printf("   📂 Directory scanning complete\n")
		close(dirQueue)
		close(fileQueue)
		// Close imageQueue only if not already closed by image completion tracker
		imageQueueCloseOnce.Do(func() {
			close(imageQueue)
		})
	}()

	// Wait for all processing to complete
	fileWg.Wait()
	wg.Wait()
	close(errChan)

	// Check for errors
	var hasError bool
	for err := range errChan {
		if err != nil {
			hasError = true
			log.Printf("⚠️ %v", err)
		}
	}

	if hasError {
		return fmt.Errorf("some files failed to process")
	}

	return nil
}

func minifyFile(mediaType, srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	return Minifier.Minify(mediaType, dst, src)
}

func processImage(srcPath, dstPath string) error {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return err
	}

	if src.Bounds().Dx() > 1200 {
		src = imaging.Resize(src, 1200, 0, imaging.Lanczos)
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return webp.Encode(f, src, &webp.Options{Lossless: false, Quality: 80})
}

func CopyFileStandard(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	_, err = io.Copy(d, s)
	return err
}

// NormalizeCacheKey converts a file path to a normalized cache key
// Uses forward slashes for cross-platform compatibility
func NormalizeCacheKey(path string) string {
	// Convert Windows backslashes to forward slashes
	return strings.ReplaceAll(path, "\\", "/")
}

var imgRe = regexp.MustCompile(`(?i)(<img[^>]+src=["'])([^"']+)((?:\.jpg|\.jpeg|\.png))(["'])`)

func ReplaceToWebP(html string) string {
	return imgRe.ReplaceAllStringFunc(html, func(m string) string {
		parts := imgRe.FindStringSubmatch(m)
		if strings.HasPrefix(parts[2], "http") || strings.HasPrefix(parts[2], "//") {
			return m
		}
		return parts[1] + parts[2] + ".webp" + parts[4]
	})
}

func SortPosts(posts []models.PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].DateObj.Equal(posts[j].DateObj) {
			return posts[i].Title > posts[j].Title
		}
		return posts[i].DateObj.After(posts[j].DateObj)
	})
}

func GetString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func GetSlice(m map[string]interface{}, k string) []string {
	var res []string
	if v, ok := m[k]; ok {
		if l, ok := v.([]interface{}); ok {
			for _, i := range l {
				res = append(res, fmt.Sprintf("%v", i))
			}
		}
	}
	return res
}

type SocialCardCache struct {
	Hashes    map[string]string `json:"hashes"`
	GraphHash string            `json:"graph_hash"`
}

func NewSocialCardCache() *SocialCardCache {
	return &SocialCardCache{
		Hashes: make(map[string]string),
	}
}

func LoadSocialCardCache(path string) (*SocialCardCache, error) {
	cache := NewSocialCardCache()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func SaveSocialCardCache(path string, cache *SocialCardCache) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func LoadBuildCache(path string) (*models.MetadataCache, error) {
	cache := &models.MetadataCache{
		Posts: make(map[string]models.CachedPost),
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func SaveBuildCache(path string, cache *models.MetadataCache) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func GetFrontmatterHash(metaData map[string]interface{}) (string, error) {
	isPinned, _ := metaData["pinned"].(bool)
	socialMeta := map[string]interface{}{
		"title":       GetString(metaData, "title"),
		"description": GetString(metaData, "description"),
		"date":        GetString(metaData, "date"),
		"tags":        GetSlice(metaData, "tags"),
		"pinned":      isPinned,
	}

	data, err := json.Marshal(socialMeta)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type GraphHashData struct {
	Posts []PostGraphInfo `json:"posts"`
}

type PostGraphInfo struct {
	Title string   `json:"title"`
	Link  string   `json:"link"`
	Tags  []string `json:"tags"`
}

func GetGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]PostGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, PostGraphInfo{
			Title: p.Title,
			Link:  p.Link,
			Tags:  p.Tags,
		})
	}

	data, err := json.Marshal(graphInfo)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
