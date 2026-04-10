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
			buffer := make([]byte, maxResizeWidth*maxResizeHeight*rgbaBytesPerPixel)
			return &buffer
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

func validateCopyDirOptions(ctx context.Context, options CopyDirOptions) (context.Context, copyDirContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.SrcFs == nil {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: SrcFs is nil")
	}
	if options.Sink == nil {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: Sink is nil")
	}
	if options.SrcDir == "" {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: SrcDir is empty")
	}
	if options.DstDir == "" {
		return nil, copyDirContext{}, errors.New("CopyDirVFS: DstDir is empty")
	}

	directoryCtx := copyDirContext{
		srcFs:  options.SrcFs,
		sink:   options.Sink,
		srcDir: fspkg.NormalizePath(options.SrcDir),
		dstDir: fspkg.NormalizePath(options.DstDir),
	}
	return ctx, directoryCtx, nil
}

func resolveWorkerCounts(imageWorkers int) (int, int) {
	workerCount := imageWorkers
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	nonImageWorkers := max(workerCount, 2)
	if nonImageWorkers > maxNonImageWorkers {
		nonImageWorkers = maxNonImageWorkers
	}
	return workerCount, nonImageWorkers
}

func appendWorkerError(errorMutex *sync.Mutex, errorsList *[]error, err error) {
	if err == nil {
		return
	}
	errorMutex.Lock()
	*errorsList = append(*errorsList, err)
	errorMutex.Unlock()
}

func handleImageTask(ctx context.Context, directoryCtx copyDirContext, task fileTask, options CopyDirOptions, errorMutex *sync.Mutex, errorsList *[]error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Image worker panic recovered", "panic", r)
			appendWorkerError(errorMutex, errorsList, fmt.Errorf("image worker panicked on %s: %v", task.path, r))
		}
	}()

	target := filepath.Join(directoryCtx.dstDir, task.relPath)
	if err := convertToWebPVFS(ProcessImageOptions{
		Ctx:       ctx,
		SrcFs:     directoryCtx.srcFs,
		Sink:      directoryCtx.sink,
		SrcPath:   task.path,
		DstPath:   target,
		SrcInfo:   task.info,
		Opts:      options.CopyOptions,
		Scheduler: options.Scheduler,
	}); err != nil {
		appendWorkerError(errorMutex, errorsList, fmt.Errorf("failed to process image %s: %w", task.path, err))
		return
	}

	if options.OnWrite != nil {
		options.OnWrite(target)
	}
	if task.originalRelPath != "" {
		// URL format mapping - register all variants
		relativeSrc := "/" + strings.TrimPrefix(filepath.ToSlash(task.originalRelPath), "/")
		relativeDst := "/" + strings.TrimPrefix(filepath.ToSlash(task.relPath), "/")
		registerImageVariants(relativeSrc, relativeDst)
	}
}

func handleNonImageTask(directoryCtx copyDirContext, task fileTask, options CopyDirOptions, errorMutex *sync.Mutex, errorsList *[]error) {
	destPath := filepath.Join(directoryCtx.dstDir, task.relPath)
	if err := fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   directoryCtx.srcFs,
		Sink:    directoryCtx.sink,
		SrcPath: task.path,
		DstPath: destPath,
		ModTime: task.info.ModTime().UnixNano(),
		OnWrite: options.OnWrite,
	}); err != nil {
		appendWorkerError(errorMutex, errorsList, err)
		return
	}
	if options.Metrics != nil {
		options.Metrics.IncrementAssetsProcessed()
	}
}

func startImageWorkers(ctx context.Context, directoryCtx copyDirContext, options CopyDirOptions, workerCount int, errorMutex *sync.Mutex, errorsList *[]error) (chan fileTask, *sync.WaitGroup) {
	imageQueue := make(chan fileTask, fileTaskQueueSize)
	waitGroup := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		waitGroup.Add(1)
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
						handleImageTask(ctx, directoryCtx, task, options, errorMutex, errorsList)
					}
				}
			},
			Cleanup: waitGroup.Done,
		})
	}
	return imageQueue, waitGroup
}

func startNonImageWorkers(ctx context.Context, directoryCtx copyDirContext, options CopyDirOptions, workerCount int, errorMutex *sync.Mutex, errorsList *[]error) (chan fileTask, *sync.WaitGroup) {
	nonImageQueue := make(chan fileTask, fileTaskQueueSize)
	waitGroup := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		waitGroup.Add(1)
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
						handleNonImageTask(directoryCtx, task, options, errorMutex, errorsList)
					}
				}
			},
			Cleanup: waitGroup.Done,
		})
	}
	return nonImageQueue, waitGroup
}

func shouldSkipAsset(path string, options CopyDirOptions) bool {
	ext := strings.ToLower(filepath.Ext(path))
	baseName := filepath.Base(path)
	if baseName == "search.wasm" {
		return true
	}
	if baseName != "wasm_engine.js" && baseName != "engine.js" && baseName != "force-graph.js" && baseName != "wasm_exec.js" {
		if slices.Contains(options.ExcludeExts, ext) {
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

func buildWalkFn(ctx context.Context, directoryCtx copyDirContext, options CopyDirOptions, imageQueue chan<- fileTask, nonImageQueue chan<- fileTask) func(string, fs.FileInfo, error) error {
	return func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if shouldSkipAsset(path, options) {
			return nil
		}

		relPath, _ := fspkg.SafeRel(directoryCtx.srcDir, path)
		ext := strings.ToLower(filepath.Ext(path))
		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")
		finalRelPath := relPath
		if options.Compress && isImage {
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
func CopyDirVFS(ctx context.Context, options CopyDirOptions) error {
	ctx, directoryCtx, err := validateCopyDirOptions(ctx, options)
	if err != nil {
		return err
	}
	if err := directoryCtx.sink.MkdirAll(directoryCtx.dstDir); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", directoryCtx.dstDir, err)
	}

	workerCount, nonImageWorkers := resolveWorkerCounts(options.ImageWorkers)

	var errorsList []error
	var errorMutex sync.Mutex

	imageQueue, imageWg := startImageWorkers(ctx, directoryCtx, options, workerCount, &errorMutex, &errorsList)
	nonImageQueue, nonImageWg := startNonImageWorkers(ctx, directoryCtx, options, nonImageWorkers, &errorMutex, &errorsList)

	// Use higher concurrency for discovery walk on modern SSDs
	walkConcurrency := max(workerCount/2, minWalkConcurrency)
	walkFn := buildWalkFn(ctx, directoryCtx, options, imageQueue, nonImageQueue)
	walkErr := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         ctx,
		SourceFs:    directoryCtx.srcFs,
		Root:        directoryCtx.srcDir,
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
	if len(errorsList) > 0 {
		return errorsList[0]
	}

	return nil
}
