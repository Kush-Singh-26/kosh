package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/fsnotify/fsnotify"
)

var (
	watcher        *fsnotify.Watcher
	watcherMu      sync.RWMutex // Protects watcher variable
	reloadChan     chan string
	reloadMu       sync.RWMutex
	watcherWg      sync.WaitGroup
	debounceConfig time.Duration
	timerMu        sync.Mutex
	debounceTimer  *time.Timer

	// Build coordination
	buildMu       sync.Mutex
	buildActive   bool
	buildWaitChan chan struct{}

	// Watch directory mapping
	watchTargets   = make(map[string]string) // Normalized path -> target
	watchTargetsMu sync.RWMutex

	// Exclusions for active event filtering
	watchExclusions   []string
	watchExclusionsMu sync.RWMutex
)

func isPathExcluded(path string) bool {
	absPath, err := fspkg.AbsNormalizePath(path)
	if err != nil {
		return false
	}

	watchExclusionsMu.RLock()
	defer watchExclusionsMu.RUnlock()

	for _, ex := range watchExclusions {
		if fspkg.IsPathInOrSame(absPath, ex) {
			return true
		}
	}
	return false
}

// SetBuildActive marks the build as active or inactive
func SetBuildActive(active bool) {
	buildMu.Lock()
	defer buildMu.Unlock()

	if active {
		if !buildActive {
			buildActive = true
			buildWaitChan = make(chan struct{})
		}
	} else {
		if buildActive {
			buildActive = false
			if buildWaitChan != nil {
				close(buildWaitChan)
				buildWaitChan = nil
			}
		}
	}
}

// waitForBuild returns a channel that will be closed when the current build completes.
// If no build is active, it returns nil.
func waitForBuild() chan struct{} {
	buildMu.Lock()
	defer buildMu.Unlock()
	return buildWaitChan
}

// resetDebounceTimer safely stops and resets the debounce timer.
func resetDebounceTimer(target, path string) {
	timerMu.Lock()
	defer timerMu.Unlock()

	if debounceTimer != nil {
		debounceTimer.Stop()
	}

	debounceTimer = time.AfterFunc(debounceConfig, func() {
		reloadMu.RLock()
		defer reloadMu.RUnlock()
		if reloadChan == nil {
			return
		}
		select {
		case reloadChan <- target + ":" + path:
		default:
		}
	})
}

type watchConfig struct {
	Dirs       map[string]string // dir -> target
	Debounce   time.Duration
	Exclusions []string
}

func startWatcherWithConfig(cfg watchConfig) chan string {
	debounceConfig = cfg.Debounce

	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("Failed to create file watcher", "error", err)
		return nil
	}

	initWatchExclusions(cfg.Exclusions)
	initWatchTargets(w, cfg.Dirs)

	watcherMu.Lock()
	watcher = w
	watcherMu.Unlock()

	var currentReloadChan chan string
	reloadMu.Lock()
	reloadChan = make(chan string, 1)
	currentReloadChan = reloadChan
	reloadMu.Unlock()

	startWatcherLoop(w)

	return currentReloadChan
}

func initWatchExclusions(exclusions []string) {
	watchExclusionsMu.Lock()
	defer watchExclusionsMu.Unlock()
	watchExclusions = make([]string, 0, len(exclusions))
	for _, ex := range exclusions {
		if abs, err := fspkg.AbsNormalizePath(ex); err == nil {
			watchExclusions = append(watchExclusions, abs)
		}
	}
}

func initWatchTargets(w *fsnotify.Watcher, dirs map[string]string) {
	watchTargetsMu.Lock()
	defer watchTargetsMu.Unlock()
	for dir, target := range dirs {
		absDir, _ := fspkg.AbsNormalizePath(dir)
		watchTargets[absDir] = target

		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			absPath, _ := fspkg.AbsNormalizePath(path)
			// Never exclude the watch root itself, even if it matches an exclusion pattern
			// This allows us to watch the site-root but exclude subdirectories.
			if absPath != absDir && isPathExcluded(absPath) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if err := w.Add(path); err != nil {
					slog.Warn("Failed to watch directory", "dir", path, "error", err)
				}
			}
			return nil
		})
		if err != nil {
			slog.Warn("Failed to walk directory for watching", "dir", dir, "error", err)
		}
	}
}

func startWatcherLoop(w *fsnotify.Watcher) {
	watcherWg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       context.Background(),
		Logger:    slog.Default(),
		Operation: "file watcher",
		Fn: func() error {
			defer func() {
				if w != nil {
					_ = w.Close()
				}
			}()

			for {
				select {
				case event, ok := <-w.Events:
					if !ok {
						return nil
					}
					if event.Op&fsnotify.Chmod != 0 {
						continue
					}

					// Skip excluded paths in the active event loop
					if isPathExcluded(event.Name) {
						continue
					}

					target := resolveTarget(event.Name)
					resetDebounceTimer(target, event.Name)

				case err, ok := <-w.Errors:
					if !ok {
						return nil
					}
					slog.Warn("Watcher error", "error", err)
				}
			}
		},
		Cleanup: watcherWg.Done,
	})
}

func resolveTarget(path string) string {
	absPath, err := fspkg.AbsNormalizePath(path)
	if err != nil {
		return "site"
	}

	watchTargetsMu.RLock()
	defer watchTargetsMu.RUnlock()

	bestTarget := "site"
	maxLen := -1

	for dir, target := range watchTargets {
		if fspkg.IsPathInOrSame(absPath, dir) {
			if len(dir) > maxLen {
				maxLen = len(dir)
				bestTarget = target
			}
		}
	}
	return bestTarget
}

func stopWatcher() {
	timerMu.Lock()
	if debounceTimer != nil {
		debounceTimer.Stop()
	}
	timerMu.Unlock()

	watcherMu.Lock()
	w := watcher
	watcher = nil
	watcherMu.Unlock()

	if w != nil {
		_ = w.Close()
	}
	watcherWg.Wait()

	reloadMu.Lock()
	if reloadChan == nil {
		reloadMu.Unlock()
		return
	}
	close(reloadChan)
	reloadChan = nil
	reloadMu.Unlock()
}
