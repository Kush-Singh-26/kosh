package fs

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
)

// WalkFunc is the signature for the callback used in ParallelWalk.
type WalkFunc func(path string, info fs.FileInfo, err error) error

// ParallelWalk provides a stable, parallelized directory traversal using the afero interface.
func ParallelWalk(ctx context.Context, sourceFs afero.Fs, root string, concurrency int, walkFn WalkFunc) error {
	// Add context cancellation to the walk function
	walkFnWrapped := func(path string, info fs.FileInfo, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return walkFn(path, info, err)
	}

	// Root processing (parity with filepath.Walk)
	rootInfo, err := sourceFs.Stat(root)
	if err := walkFnWrapped(root, rootInfo, err); err != nil {
		if errors.Is(err, filepath.SkipDir) || errors.Is(err, fs.SkipAll) {
			return nil
		}
		return err
	}

	if rootInfo == nil || !rootInfo.IsDir() {
		return nil
	}

	if concurrency <= 0 {
		concurrency = 4 // Safe default for overlapping I/O latency
	}

	type dirTask struct {
		path string
	}

	tasks := make(chan dirTask, 4096)
	var wg sync.WaitGroup
	var activeTasks int32
	var firstErr error
	var errOnce sync.Once
	var cancelOnce sync.Once

	setErr := func(err error) {
		if err != nil {
			errOnce.Do(func() {
				firstErr = err
			})
		}
	}

	// Initial task
	atomic.AddInt32(&activeTasks, 1)
	tasks <- dirTask{path: root}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case t, ok := <-tasks:
					if !ok {
						return
					}

					// Read directory entries in bulk
					entries, err := afero.ReadDir(sourceFs, t.path)
					if err != nil {
						setErr(walkFnWrapped(t.path, nil, err))
					} else {
						for _, entry := range entries {
							if ctx.Err() != nil || firstErr != nil {
								break
							}

							fullPath := filepath.ToSlash(filepath.Join(t.path, entry.Name()))

							// Execute callback
							walkErr := walkFnWrapped(fullPath, entry, nil)
							if walkErr != nil {
								if errors.Is(walkErr, filepath.SkipDir) {
									if entry.IsDir() {
										continue
									}
									break
								}
								if errors.Is(walkErr, fs.SkipAll) {
									setErr(walkErr)
									cancelOnce.Do(func() {
										// Cancel context to stop other workers
									})
									break
								}
								setErr(walkErr)
								break
							}

							if entry.IsDir() {
								atomic.AddInt32(&activeTasks, 1)
								go func(p string) {
									select {
									case tasks <- dirTask{path: p}:
									case <-ctx.Done():
										atomic.AddInt32(&activeTasks, -1)
									}
								}(fullPath)
							}
						}
					}

					// Check if we are done
					if atomic.AddInt32(&activeTasks, -1) == 0 {
						close(tasks)
					}
				}
			}
		}()
	}

	wg.Wait()
	if errors.Is(firstErr, fs.SkipAll) {
		return nil
	}
	return firstErr
}
