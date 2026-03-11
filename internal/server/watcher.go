package server

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	watcher        *fsnotify.Watcher
	watcherMu      sync.RWMutex // Protects watcher variable
	reloadChan     chan struct{}
	reloadMu       sync.RWMutex
	clientMu       sync.Mutex
	clients        = make(map[chan struct{}]struct{})
	watcherWg      sync.WaitGroup
	debounceConfig time.Duration
	timerMu        sync.Mutex
	debounceTimer  *time.Timer

	// Build coordination
	buildMu       sync.Mutex
	buildActive   bool
	buildWaitChan chan struct{}
)

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
// This function is thread-safe and must be called while holding timerMu.
func resetDebounceTimer() {
	timerMu.Lock()
	defer timerMu.Unlock()

	if debounceTimer != nil {
		if !debounceTimer.Stop() {
			// Timer already fired, drain the channel
			select {
			case <-debounceTimer.C:
			default:
			}
		}
	}

	debounceTimer = time.AfterFunc(debounceConfig, func() {
		reloadMu.RLock()
		defer reloadMu.RUnlock()
		if reloadChan == nil {
			return
		}
		select {
		case reloadChan <- struct{}{}:
		default:
		}
	})
}

func startWatcherWithConfig(dir string, debounce time.Duration) chan struct{} {
	debounceConfig = debounce

	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("Failed to create file watcher", "error", err)
		return nil
	}

	if err := w.Add(dir); err != nil {
		slog.Warn("Failed to watch directory", "dir", dir, "error", err)
		_ = w.Close()
		return nil
	}

	watcherMu.Lock()
	watcher = w
	watcherMu.Unlock()

	reloadMu.Lock()
	reloadChan = make(chan struct{}, 1)
	currentReloadChan := reloadChan
	reloadMu.Unlock()

	watcherWg.Add(1)
	go func() {
		defer watcherWg.Done()
		defer func() {
			watcherMu.RLock()
			w := watcher
			watcherMu.RUnlock()
			if w != nil {
				if err := w.Close(); err != nil {
					slog.Warn("Failed to close file watcher", "error", err)
				}
			}
		}()

		watcherMu.RLock()
		w := watcher
		watcherMu.RUnlock()

		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Chmod != 0 {
					continue
				}

				// Safely reset the debounce timer
				resetDebounceTimer()

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Warn("Watcher error", "error", err)
			}
		}
	}()

	return currentReloadChan
}

func stopWatcher() {
	// Stop the debounce timer to prevent goroutine leaks
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
		if err := w.Close(); err != nil {
			slog.Warn("Failed to close file watcher", "error", err)
		}
	}
	watcherWg.Wait()

	reloadMu.Lock()
	if reloadChan != nil {
		close(reloadChan)
		reloadChan = nil
	}
	reloadMu.Unlock()
}
