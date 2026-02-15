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

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/spf13/afero"
	"github.com/zeebo/blake3"
)

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
	if err == nil {
		if dstInfo, err := os.Stat(dstPath); err == nil {
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				data, err := os.ReadFile(dstPath)
				if err == nil {
					return afero.WriteFile(destFs, dstPath, data, 0644)
				}
			}
		}
	}

	var cacheFile string
	if cacheDir != "" && err == nil {
		key := fmt.Sprintf("%s-%d-%d", srcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())
		hash := blake3.Sum256([]byte(key))
		hashStr := hex.EncodeToString(hash[:])
		cacheFile = filepath.Join(cacheDir, hashStr+".webp")

		if data, err := os.ReadFile(cacheFile); err == nil {
			return WriteFileVFS(destFs, dstPath, data)
		}
	}

	file, err := srcFs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source image %s: %w", srcPath, err)
	}
	defer func() { _ = file.Close() }()

	img, err := imaging.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %w", srcPath, err)
	}

	if img.Bounds().Dx() > 1200 {
		img = imaging.Resize(img, 1200, 0, imaging.Lanczos)
	}

	if err := destFs.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	if cacheFile != "" {
		fCache, err := os.Create(cacheFile)
		if err == nil {
			err = webp.Encode(fCache, img, &webp.Options{Lossless: false, Quality: 80})
			if cerr := fCache.Close(); cerr != nil {
				slog.Warn("Failed to close cache file", "path", cacheFile, "error", cerr)
			}
			if err == nil {
				data, err := os.ReadFile(cacheFile)
				if err == nil {
					return afero.WriteFile(destFs, dstPath, data, 0644)
				}
			}
		}
	}

	out, err := destFs.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination image %s: %w", dstPath, err)
	}
	defer func() { _ = out.Close() }()

	if err := webp.Encode(out, img, &webp.Options{Lossless: false, Quality: 80}); err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", dstPath, err)
	}
	return nil
}
