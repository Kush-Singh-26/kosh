package assets

import (
	"context"
	"errors"
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

const (
	maxNonImageWorkers = 32
	fileTaskQueueSize  = 1024
	minWalkConcurrency = 4
)

type fileTask struct {
	path            string
	relPath         string
	originalRelPath string
	info            fs.FileInfo
}

type copyDirContext struct {
	srcFs  afero.Fs
	sink   fspkg.ArtifactSink
	srcDir string
	dstDir string
}

var (
	// rgbaPixPool stores *[]byte buffers sized for maxResizeWidth x maxResizeHeight RGBA images.
	rgbaPixPool = sync.Pool{
		New: func() any {
			b := make([]byte, maxResizeWidth*maxResizeHeight*rgbaBytesPerPixel)
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

func validateCopyDirOptions(ctx context.Context, opts CopyDirOptions) (context.Context, copyDirContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.SrcFs == nil {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: SrcFs is nil")
	}
	if opts.Sink == nil {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: Sink is nil")
	}
	if opts.SrcDir == "" {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: SrcDir is empty")
	}
	if opts.DstDir == "" {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: DstDir is empty")
	}

	c := copyDirContext{
		srcFs:  opts.SrcFs,
		sink:   opts.Sink,
		srcDir: fspkg.NormalizePath(opts.SrcDir),
		dstDir: fspkg.NormalizePath(opts.DstDir),
	}
	return ctx, c, nil
}

func resolveWorkerCounts(imageWorkers int) (int, int) {
	numWorkers := imageWorkers
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	nonImageWorkers := max(numWorkers, 2)
	if nonImageWorkers > maxNonImageWorkers {
		nonImageWorkers = maxNonImageWorkers
	}
	return numWorkers, nonImageWorkers
}

func appendWorkerError(errMu *sync.Mutex, errs *[]error, err error) {
	if err == nil {
		return
	}
	errMu.Lock()
	*errs = append(*errs, err)
	errMu.Unlock()
}

func handleImageTask(ctx context.Context, c copyDirContext, task fileTask, opts CopyDirOptions, errMu *sync.Mutex, errs *[]error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Image worker panic recovered", "panic", r)
			appendWorkerError(errMu, errs, fmt.Errorf("image worker panicked on %s: %v", task.path, r))
		}
	}()

	target := filepath.Join(c.dstDir, task.relPath)
	if err := convertToWebPVFS(ProcessImageOptions{
		Ctx:       ctx,
		SrcFs:     c.srcFs,
		Sink:      c.sink,
		SrcPath:   task.path,
		DstPath:   target,
		SrcInfo:   task.info,
		Opts:      opts.CopyOptions,
		Scheduler: opts.Scheduler,
	}); err != nil {
		appendWorkerError(errMu, errs, fmt.Errorf("failed to process image %s: %w", task.path, err))
		return
	}

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

func handleNonImageTask(c copyDirContext, task fileTask, opts CopyDirOptions, errMu *sync.Mutex, errs *[]error) {
	destPath := filepath.Join(c.dstDir, task.relPath)
	if err := fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   c.srcFs,
		Sink:    c.sink,
		SrcPath: task.path,
		DstPath: destPath,
		ModTime: task.info.ModTime().UnixNano(),
		OnWrite: opts.OnWrite,
	}); err != nil {
		appendWorkerError(errMu, errs, err)
		return
	}
	if opts.Metrics != nil {
		opts.Metrics.IncrementAssetsProcessed()
	}
}

func startImageWorkers(ctx context.Context, c copyDirContext, opts CopyDirOptions, numWorkers int, errMu *sync.Mutex, errs *[]error) (chan fileTask, *sync.WaitGroup) {
	imageQueue := make(chan fileTask, fileTaskQueueSize)
	wg := &sync.WaitGroup{}
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
						handleImageTask(ctx, c, task, opts, errMu, errs)
					}
				}
			},
			Cleanup: wg.Done,
		})
	}
	return imageQueue, wg
}

func startNonImageWorkers(ctx context.Context, c copyDirContext, opts CopyDirOptions, numWorkers int, errMu *sync.Mutex, errs *[]error) (chan fileTask, *sync.WaitGroup) {
	nonImageQueue := make(chan fileTask, fileTaskQueueSize)
	wg := &sync.WaitGroup{}
	for i := 0; i < numWorkers; i++ {
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
						handleNonImageTask(c, task, opts, errMu, errs)
					}
				}
			},
			Cleanup: wg.Done,
		})
	}
	return nonImageQueue, wg
}

func shouldSkipAsset(path string, opts CopyDirOptions) bool {
	ext := strings.ToLower(filepath.Ext(path))
	baseName := filepath.Base(path)
	if baseName == "search.wasm" {
		return true
	}
	if baseName != "wasm_engine.js" && baseName != "engine.js" && baseName != "force-graph.js" && baseName != "wasm_exec.js" {
		if slices.Contains(opts.ExcludeExts, ext) {
			return true
		}
	}
	return false
}

func enqueueTask(ctx context.Context, queue chan<- fileTask, task fileTask) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case queue <- task:
		return nil
	}
}

func buildWalkFn(ctx context.Context, c copyDirContext, opts CopyDirOptions, imageQueue chan<- fileTask, nonImageQueue chan<- fileTask) func(string, fs.FileInfo, error) error {
	return func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if shouldSkipAsset(path, opts) {
			return nil
		}

		relPath, _ := fspkg.SafeRel(c.srcDir, path)
		ext := strings.ToLower(filepath.Ext(path))
		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")
		finalRelPath := relPath
		if opts.Compress && isImage {
			finalRelPath = relPath[:len(relPath)-len(ext)] + ".webp"
			return enqueueTask(ctx, imageQueue, fileTask{
				path:            path,
				relPath:         finalRelPath,
				originalRelPath: relPath,
				info:            info,
			})
		}

		return enqueueTask(ctx, nonImageQueue, fileTask{
			path:            path,
			relPath:         finalRelPath,
			originalRelPath: "",
			info:            info,
		})
	}
}

// CopyDirVFS copies assets from a source VFS into the artifact sink.
func CopyDirVFS(ctx context.Context, opts CopyDirOptions) error {
	ctx, c, err := validateCopyDirOptions(ctx, opts)
	if err != nil {
		return err
	}
	if err := c.sink.MkdirAll(c.dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", c.dstDir, err)
	}

	numWorkers, nonImageWorkers := resolveWorkerCounts(opts.ImageWorkers)

	var errs []error
	var errMu sync.Mutex

	imageQueue, imageWg := startImageWorkers(ctx, c, opts, numWorkers, &errMu, &errs)
	nonImageQueue, nonImageWg := startNonImageWorkers(ctx, c, opts, nonImageWorkers, &errMu, &errs)

	// Use higher concurrency for discovery walk on modern SSDs
	walkConcurrency := max(numWorkers/2, minWalkConcurrency)
	walkFn := buildWalkFn(ctx, c, opts, imageQueue, nonImageQueue)
	walkErr := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         ctx,
		SourceFs:    c.srcFs,
		Root:        c.srcDir,
		Concurrency: walkConcurrency,
		WalkFn:      walkFn,
	})

	close(imageQueue)
	close(nonImageQueue)
	imageWg.Wait()
	nonImageWg.Wait()

	if walkErr != nil {
		return walkErr
	}
	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}
