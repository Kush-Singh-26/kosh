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

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/async"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

type fileTask struct {
	path            string
	relPath         string
	originalRelPath string
	info            fs.FileInfo
}

var (
	// rgbaPixPool stores *[]byte buffers sized for 1200x1600 RGBA images.
	rgbaPixPool = sync.Pool{
		New: func() any {
			b := make([]byte, 1200*1600*4)
			return &b
		},
	}
)

// CopyOptions controls how assets are copied and processed.
type CopyOptions struct {
	Compress     bool
	MinifySVGs   bool
	KeepOriginal bool
	ExcludeExts  []string
	OnWrite      func(string)
	CacheDir     string
	ImageWorkers int
	WebPQuality  int
	Metrics      ImageMetrics
	Scheduler    scheduler.BuildScheduler
}

// CopyDirOptions configures CopyDirVFS.
type CopyDirOptions struct {
	// Required
	SrcFs  afero.Fs
	Sink   fspkg.ArtifactSink
	SrcDir string
	DstDir string

	// Optional
	CopyOptions
}

// CopyDirVFS copies assets from a source VFS into the artifact sink.
func CopyDirVFS(ctx context.Context, opts CopyDirOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.SrcFs == nil {
		return fmt.Errorf("CopyDirVFS: SrcFs is nil")
	}
	if opts.Sink == nil {
		return fmt.Errorf("CopyDirVFS: Sink is nil")
	}
	if opts.SrcDir == "" {
		return fmt.Errorf("CopyDirVFS: SrcDir is empty")
	}
	if opts.DstDir == "" {
		return fmt.Errorf("CopyDirVFS: DstDir is empty")
	}

	srcFs := opts.SrcFs
	sink := opts.Sink
	srcDir := fspkg.NormalizePath(opts.SrcDir)
	dstDir := fspkg.NormalizePath(opts.DstDir)
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
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "asset image worker",
			Fn: func() error {
				for {
					select {
					case <-ctx.Done():
						return nil
					case task, ok := <-imageQueue:
						if !ok {
							return nil
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
								Opts:      opts.CopyOptions,
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
									// URL format mapping - register all variants
									relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(task.originalRelPath), "/")
									relDst := "/" + strings.TrimPrefix(filepath.ToSlash(task.relPath), "/")
									registerImageVariants(relSrc, relDst)
								}
							}
						}()
					}
				}
			},
			Cleanup: wg.Done,
		})
	}
	for i := 0; i < nonImageWorkers; i++ {
		wg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "asset copy worker",
			Fn: func() error {
				for {
					select {
					case <-ctx.Done():
						return nil
					case task, ok := <-nonImageQueue:
						if !ok {
							return nil
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
			},
			Cleanup: wg.Done,
		})
	}

	// Use higher concurrency for discovery walk on modern SSDs
	walkConcurrency := max(numWorkers/2, 4)
	walkErr := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         ctx,
		SourceFs:    srcFs,
		Root:        srcDir,
		Concurrency: walkConcurrency,
		WalkFn: func(path string, info fs.FileInfo, err error) error {
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
		},
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
