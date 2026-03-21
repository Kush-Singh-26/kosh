//go:build !wasm

package fs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/spf13/afero"
)

type fileTask struct {
	path            string
	relPath         string
	originalRelPath string
	info            fs.FileInfo
}

var (
	copyBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}

	rgbaPixPool = sync.Pool{
		New: func() any {
			b := make([]byte, 1200*1600*4)
			return &b
		},
	}
)

type CopyOptions struct {
	Compress     bool
	MinifySVGs   bool
	ExcludeExts  []string
	OnWrite      func(string)
	CacheDir     string
	ImageWorkers int
	WebPQuality  int
	Metrics      ImageMetrics
	Scheduler    scheduler.BuildScheduler
}

type CopyFileOptions struct {
	SrcFs   afero.Fs
	Sink    ArtifactSink
	SrcPath string
	DstPath string
	ModTime int64
	OnWrite func(string)
}

func CopyFileVFS(opts CopyFileOptions) error {
	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(opts.DstPath), err)
	}

	// Attempt optimized syscall copy for local files
	if realSrc, ok := GetRealPath(opts.SrcFs, opts.SrcPath); ok {
		err := opts.Sink.CopyFile(realSrc, opts.DstPath)
		if err == nil {
			if opts.OnWrite != nil {
				opts.OnWrite(opts.DstPath)
			}
			if opts.ModTime > 0 {
				_ = opts.Sink.SetMtime(opts.DstPath, time.Unix(0, opts.ModTime))
			}
			return nil
		}
		// Fallback to streaming if optimized copy fails
		slog.Debug("Optimized copy failed, falling back to streaming", "path", opts.SrcPath, "error", err)
	}

	in, err := opts.SrcFs.Open(opts.SrcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", opts.SrcPath, err)
	}
	defer func() { _ = in.Close() }()

	bufPtr := copyBufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufferPool.Put(bufPtr)

	errWrite := opts.Sink.WriteStream(opts.DstPath, func(w io.Writer) error {
		_, err := io.CopyBuffer(w, in, buf)
		return err
	})
	if errWrite != nil {
		return fmt.Errorf("failed to copy file %s: %w", opts.SrcPath, errWrite)
	}

	if opts.OnWrite != nil {
		opts.OnWrite(opts.DstPath)
	}

	if opts.ModTime > 0 {
		_ = opts.Sink.SetMtime(opts.DstPath, time.Unix(0, opts.ModTime))
	}

	return nil
}

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, sink ArtifactSink, srcDir, dstDir string, opts CopyOptions) error {
	srcDir = NormalizePath(srcDir)
	dstDir = NormalizePath(dstDir)
	if err := sink.MkdirAll(dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstDir, err)
	}

	numWorkers := opts.ImageWorkers
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
								errMu.Lock()
								errs = append(errs, fmt.Errorf("image worker panicked on %s: %v", task.path, r))
								errMu.Unlock()
							}
						}()
						target := filepath.Join(dstDir, task.relPath)
						if err := convertToWebPVFS(ProcessImageOptions{
							Ctx:       ctx,
							SrcFs:     srcFs,
							Sink:      sink,
							SrcPath:   task.path,
							DstPath:   target,
							SrcInfo:   task.info,
							Opts:      opts,
							Scheduler: opts.Scheduler,
						}); err != nil {
							errMu.Lock()
							errs = append(errs, fmt.Errorf("failed to process image %s: %w", task.path, err))
							errMu.Unlock()
						} else {
							if opts.OnWrite != nil {
								opts.OnWrite(target)
							}
							if task.originalRelPath != "" {
								// File system path mapping
								RecordConvertedImage(filepath.Join(dstDir, task.originalRelPath), target)
								// URL format mapping (ensure leading slash)
								relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(task.originalRelPath), "/")
								relDst := "/" + strings.TrimPrefix(filepath.ToSlash(task.relPath), "/")
								RecordConvertedImage(relSrc, relDst)
							}
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
					if err := CopyFileVFS(CopyFileOptions{
						SrcFs:   srcFs,
						Sink:    sink,
						SrcPath: task.path,
						DstPath: destPath,
						ModTime: task.info.ModTime().UnixNano(),
						OnWrite: opts.OnWrite,
					}); err != nil {
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
			if slices.Contains(opts.ExcludeExts, ext) {
				isExcluded = true
			}
		}
		if isExcluded {
			return nil
		}

		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")
		finalRelPath := relPath
		if opts.Compress && isImage {
			finalRelPath = relPath[:len(relPath)-len(ext)] + ".webp"
			select {
			case <-ctx.Done():
				return ctx.Err()
			case imageQueue <- fileTask{
				path:            path,
				relPath:         finalRelPath,
				originalRelPath: relPath,
				info:            info,
			}:
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case nonImageQueue <- fileTask{
				path:            path,
				relPath:         finalRelPath,
				originalRelPath: "",
				info:            info,
			}:
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
