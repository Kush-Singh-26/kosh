package utils

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
	"github.com/zeebo/blake3"
)

// NormalizePath normalizes a file path for consistent cache keys and cross-platform compatibility.
// Uses forward slashes, removes content/ prefix, converts to lowercase, and handles Windows drive letters.
// Optimized to reduce allocations using strings.Builder.
func NormalizePath(path string) string {
	// Fast path: no content/ prefix and no backslashes
	if !strings.Contains(path, "\\") && !strings.HasPrefix(path, "content/") {
		return strings.ToLower(path)
	}

	var b strings.Builder
	b.Grow(len(path))

	// Normalize separators and remove prefix in one pass
	skipContent := strings.HasPrefix(path, "content/") || strings.HasPrefix(path, "content\\")
	start := 0
	if skipContent {
		start = 8 // len("content/")
	}

	for i := start; i < len(path); i++ {
		c := path[i]
		if c == '\\' {
			b.WriteByte('/')
		} else if c >= 'A' && c <= 'Z' {
			b.WriteByte(c + 32) // ToLower without function call
		} else {
			b.WriteByte(c)
		}
	}

	result := b.String()

	// Handle Windows drive letter casing if present
	if runtime.GOOS == "windows" && len(result) >= 2 && result[1] == ':' {
		return strings.ToUpper(result[:1]) + result[1:]
	}

	return result
}

// SafeRel is a wrapper around filepath.Rel that normalizes paths first to ensure consistency.
func SafeRel(base, target string) (string, error) {
	base = filepath.FromSlash(NormalizePath(base))
	target = filepath.FromSlash(NormalizePath(target))
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// CopyDirVFS copies a directory from srcFs to destFs with parallel image processing.
// imageWorkers specifies the number of parallel workers for image processing (0 uses runtime.NumCPU()).
func CopyDirVFS(srcFs afero.Fs, destFs afero.Fs, srcDir, dstDir string, compress bool, excludeExts []string, onWrite func(string), cacheDir string, imageWorkers int) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	// Create destination directory
	if err := destFs.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	type fileTask struct {
		path    string
		relPath string
		info    fs.FileInfo
	}

	imageQueue := make(chan fileTask, 100)
	errChan := make(chan error, 100)
	var wg sync.WaitGroup

	// Start Image Workers
	numWorkers := imageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range imageQueue {
				target := filepath.Join(dstDir, task.relPath)
				if err := processImageVFS(srcFs, destFs, task.path, target, cacheDir); err != nil {
					errChan <- fmt.Errorf("failed to process image %s: %w", task.path, err)
				} else if onWrite != nil {
					onWrite(target)
				}
			}
		}()
	}

	// Walk Source
	err := afero.Walk(srcFs, srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := SafeRel(srcDir, path)
		destPath := filepath.Join(dstDir, relPath)

		ext := strings.ToLower(filepath.Ext(path))

		// Check exclusions
		for _, exclude := range excludeExts {
			if ext == exclude {
				return nil
			}
		}

		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

		if compress && isImage {
			// Queue for processing
			// Adjust relPath for the task to match the webp destination
			relPathWebP := relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".webp"
			imageQueue <- fileTask{path, relPathWebP, info}
		} else {
			// Direct Copy
			destDir := filepath.Dir(destPath)
			if err := destFs.MkdirAll(destDir, 0755); err != nil {
				return err
			}

			in, err := srcFs.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = in.Close() }()

			out, err := destFs.Create(destPath)
			if err != nil {
				return err
			}
			defer func() { _ = out.Close() }()

			if _, err := io.Copy(out, in); err != nil {
				return err
			}
			if onWrite != nil {
				onWrite(destPath)
			}
		}
		return nil
	})

	close(imageQueue)
	wg.Wait()
	close(errChan)

	if err != nil {
		return err
	}

	for err := range errChan {
		if err != nil {
			return err // Return first error
		}
	}

	return nil
}

func processImageVFS(srcFs afero.Fs, destFs afero.Fs, srcPath, dstPath string, cacheDir string) error {
	// Skip if destination exists and is newer than source
	srcInfo, err := srcFs.Stat(srcPath)
	if err == nil {
		if dstInfo, err := os.Stat(dstPath); err == nil {
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				// Optimization: Read existing from disk into VFS to keep it consistent
				data, err := os.ReadFile(dstPath)
				if err == nil {
					return afero.WriteFile(destFs, dstPath, data, 0644)
				}
			}
		}
	}

	// Persistent Cache Check
	var cacheFile string
	if cacheDir != "" && err == nil {
		key := fmt.Sprintf("%s-%d-%d", srcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
		hash := blake3.Sum256([]byte(key))
		hashStr := hex.EncodeToString(hash[:])
		cacheFile = filepath.Join(cacheDir, hashStr+".webp")

		if data, err := os.ReadFile(cacheFile); err == nil {
			// Cache Hit! Write to VFS
			return WriteFileVFS(destFs, dstPath, data)
		}
	}

	// Read
	file, err := srcFs.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	img, err := imaging.Decode(file)
	if err != nil {
		return err
	}

	// Resize if needed
	if img.Bounds().Dx() > 1200 {
		img = imaging.Resize(img, 1200, 0, imaging.Lanczos)
	}

	// Write
	if err := destFs.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	if cacheFile != "" {
		// Write to persistent cache first
		fCache, err := os.Create(cacheFile)
		if err == nil {
			err = webp.Encode(fCache, img, &webp.Options{Lossless: false, Quality: 80})
			fCache.Close()
			if err == nil {
				// Copy from cache to VFS
				data, err := os.ReadFile(cacheFile)
				if err == nil {
					return afero.WriteFile(destFs, dstPath, data, 0644)
				}
			}
		}
		// Fallback if cache write fails
	}

	out, err := destFs.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	return webp.Encode(out, img, &webp.Options{Lossless: false, Quality: 80})
}

// WriteFileVFS helper
func WriteFileVFS(fs afero.Fs, path string, data []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return afero.WriteFile(fs, path, data, 0644)
}

// HydrateVFS populates a VFS from a directory on disk.
func HydrateVFS(vfs afero.Fs, diskDir string) error {
	return filepath.Walk(diskDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return vfs.MkdirAll(path, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return afero.WriteFile(vfs, path, data, 0644)
	})
}

// HashDirs generates a deterministic BLAKE3 hash of multiple directories' contents.
// It uses file content for accuracy.
func HashDirs(dirs []string) (string, error) {
	h := blake3.New()
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashDirsFast generates a deterministic BLAKE3 hash based on file metadata (path, size, mtime).
// This is much faster than reading content but relies on filesystem timestamps.
func HashDirsFast(dirs []string) (string, error) {
	h := blake3.New()
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Write metadata to hash
			fmt.Fprintf(h, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFiles generates a deterministic BLAKE3 hash of multiple files.
func HashFiles(files []string) (string, error) {
	sort.Strings(files)
	h := blake3.New()
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", err
		}
		_ = f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
