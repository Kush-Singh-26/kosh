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
	clientMu       sync.Mutex
	clients        = make(map[chan struct{}]struct{})
	watcherWg      sync.WaitGroup
	debounceConfig time.Duration
	timerMu        sync.Mutex
	debounceTimer  *time.Timer

	// Build coordination
	buildMu       sync.Mutex
	buildActive   bool
	buildComplete *sync.Cond
)

func init() {
	buildComplete = sync.NewCond(&buildMu)
}

// SetBuildActive marks the build as active or inactive
func SetBuildActive(active bool) {
	buildMu.Lock()
	buildActive = active
	if !active {
		buildComplete.Broadcast()
	}
	buildMu.Unlock()
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
		select {
		case reloadChan <- struct{}{}:
		default:
		}
	})
}

func startWatcherWithConfig(dir string, debounce time.Duration) {
	debounceConfig = debounce

	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("Failed to create file watcher", "error", err)
		return
	}

	if err := w.Add(dir); err != nil {
		slog.Warn("Failed to watch directory", "dir", dir, "error", err)
		_ = w.Close()
		return
	}

	watcherMu.Lock()
	watcher = w
	watcherMu.Unlock()

	reloadChan = make(chan struct{}, 1)

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
}
