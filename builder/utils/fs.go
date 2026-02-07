package utils

import (
	"crypto/md5"
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
)

// CopyDirVFS copies a directory from srcFs to destFs with parallel image processing.
func CopyDirVFS(srcFs afero.Fs, destFs afero.Fs, srcDir, dstDir string, compress bool, onWrite func(string)) error {
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
	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range imageQueue {
				target := filepath.Join(dstDir, task.relPath)
				if err := processImageVFS(srcFs, destFs, task.path, target); err != nil {
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

		relPath, _ := filepath.Rel(srcDir, path)
		destPath := filepath.Join(dstDir, relPath)

		ext := strings.ToLower(filepath.Ext(path))
		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

		if compress && isImage {
			// Change extension to webp
			extLen := len(filepath.Ext(destPath))
			destPath = destPath[:len(destPath)-extLen] + ".webp"

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
			defer in.Close()

			out, err := destFs.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

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

func processImageVFS(srcFs afero.Fs, destFs afero.Fs, srcPath, dstPath string) error {
	// Skip if destination exists and is newer than source
	srcInfo, err := srcFs.Stat(srcPath)
	if err == nil {
		if dstInfo, err := os.Stat(dstPath); err == nil {
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				// Optimization: Read existing from disk into VFS to keep it consistent
				// though SyncVFS might skip it later anyway if it sees it on disk.
				data, err := os.ReadFile(dstPath)
				if err == nil {
					return afero.WriteFile(destFs, dstPath, data, 0644)
				}
			}
		}
	}

	// Read
	file, err := srcFs.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

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

	out, err := destFs.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

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

// HashDirs generates a deterministic MD5 hash of multiple directories' contents.
func HashDirs(dirs []string) (string, error) {
	h := md5.New()
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Use path and modtime for speed, or full content for accuracy.
			// Content is safer for WASM source hashing.
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
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

// HashFiles generates a deterministic MD5 hash of multiple files.
func HashFiles(files []string) (string, error) {
	sort.Strings(files)
	h := md5.New()
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
