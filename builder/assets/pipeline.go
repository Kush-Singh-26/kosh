package assets

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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

func CopyDirVFS(ctx context.Context, srcFs afero.Fs, sink fspkg.ArtifactSink, srcDir, dstDir string, opts CopyOptions) error {
	srcDir = fspkg.NormalizePath(srcDir)
	dstDir = fspkg.NormalizePath(dstDir)
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
					if err := fspkg.CopyFileVFS(fspkg.CopyFileOptions{
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
					} else {
						if opts.Metrics != nil {
							opts.Metrics.IncrementAssetsProcessed()
						}
					}
				}
			}
		}()
	}

	// Use higher concurrency for discovery walk on modern SSDs
	walkConcurrency := max(numWorkers/2, 4)
	walkErr := fspkg.ParallelWalk(ctx, srcFs, srcDir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := fspkg.SafeRel(srcDir, path)
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
