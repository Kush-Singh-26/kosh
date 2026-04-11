package fs

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"sync"
	"sync/atomic"

	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/spf13/afero"
)

const (
	defaultWalkConcurrency = 4
	dirTaskBufferSize      = 4096
)

// WalkFunc is the signature for the callback used in ParallelWalk.
type WalkFunc func(path string, info fs.FileInfo, err error) error

// ParallelWalk provides a stable, parallelized directory traversal using the afero interface.
type WalkOptions struct {
	Ctx         context.Context
	SourceFs    afero.Fs
	Root        string
	Concurrency int
	WalkFn      WalkFunc
}

type dirTask struct {
	path string
}

type walkState struct {
	ctx         context.Context
	sourceFs    afero.Fs
	walkFn      WalkFunc
	tasks       chan dirTask
	activeTasks int32
	firstErr    error
	errOnce     sync.Once
	cancelOnce  sync.Once
}

func wrapWalkFn(ctx context.Context, walkFn WalkFunc) WalkFunc {
	return func(path string, info fs.FileInfo, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return walkFn(path, info, err)
	}
}

func processRoot(sourceFs afero.Fs, root string, walkFn WalkFunc) (fs.FileInfo, error) {
	rootInfo, err := sourceFs.Stat(root)
	if err := walkFn(root, rootInfo, err); err != nil {
		if errors.Is(err, filepath.SkipDir) || errors.Is(err, fs.SkipAll) {
			return nil, nil
		}
		return nil, err
	}
	return rootInfo, nil
}

func (state *walkState) setErr(err error) {
	if err == nil {
		return
	}
	state.errOnce.Do(func() {
		state.firstErr = err
	})
}

func (state *walkState) maybeQueueDir(path string) {
	atomic.AddInt32(&state.activeTasks, 1)
	select {
	case state.tasks <- dirTask{path: path}:
	case <-state.ctx.Done():
		atomic.AddInt32(&state.activeTasks, -1)
	}
}

func (state *walkState) handleEntry(parent string, entry fs.FileInfo) bool {
	if state.ctx.Err() != nil || state.firstErr != nil {
		return false
	}

	fullPath := filepath.ToSlash(filepath.Join(parent, entry.Name()))
	walkErr := state.walkFn(fullPath, entry, nil)
	if walkErr != nil {
		if errors.Is(walkErr, filepath.SkipDir) {
			return !entry.IsDir()
		}
		if errors.Is(walkErr, fs.SkipAll) {
			state.setErr(walkErr)
			return false
		}
		state.setErr(walkErr)
		return false
	}

	if entry.IsDir() {
		state.maybeQueueDir(fullPath)
	}
	return true
}

func (state *walkState) processTask(task dirTask) {
	entries, err := afero.ReadDir(state.sourceFs, task.path)
	if err != nil {
		state.setErr(state.walkFn(task.path, nil, err))
	} else {
		for _, entry := range entries {
			if !state.handleEntry(task.path, entry) {
				break
			}
		}
	}

	if atomic.AddInt32(&state.activeTasks, -1) == 0 {
		close(state.tasks)
	}
}

func (state *walkState) runWorker() {
	for {
		select {
		case <-state.ctx.Done():
			return
		case task, ok := <-state.tasks:
			if !ok {
				return
			}
			state.processTask(task)
		}
	}
}

// ParallelWalk provides a stable, parallelized directory traversal using the afero interface.
func ParallelWalk(opts WalkOptions) error {
	ctx := opts.Ctx
	sourceFs := opts.SourceFs
	root := opts.Root
	concurrency := opts.Concurrency
	walkFn := opts.WalkFn

	walkFnWrapped := wrapWalkFn(ctx, walkFn)

	// Root processing (parity with filepath.Walk)
	rootInfo, err := processRoot(sourceFs, root, walkFnWrapped)
	if err != nil {
		return err
	}

	if rootInfo == nil || !rootInfo.IsDir() {
		return nil
	}

	if concurrency <= 0 {
		concurrency = defaultWalkConcurrency // Safe default for overlapping I/O latency
	}

	state := &walkState{
		ctx:      ctx,
		sourceFs: sourceFs,
		walkFn:   walkFnWrapped,
		tasks:    make(chan dirTask, dirTaskBufferSize),
	}

	// Initial task
	state.maybeQueueDir(root)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "parallel walk worker",
			Fn: func() error {
				state.runWorker()
				return nil
			},
			Cleanup: wg.Done,
		})
	}

	wg.Wait()
	if errors.Is(state.firstErr, fs.SkipAll) {
		return nil
	}
	return state.firstErr
}
