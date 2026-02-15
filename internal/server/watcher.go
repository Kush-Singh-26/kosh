package server

import (
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	watcher        *fsnotify.Watcher
	reloadChan     chan struct{}
	clientMu       sync.Mutex
	clients        = make(map[chan struct{}]struct{})
	watcherWg      sync.WaitGroup
	debounceConfig time.Duration
)

func startWatcherWithConfig(dir string, debounce time.Duration) {
	debounceConfig = debounce
	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create file watcher: %v", err)
		return
	}

	if err := watcher.Add(dir); err != nil {
		log.Printf("Failed to watch directory %s: %v", dir, err)
		return
	}

	reloadChan = make(chan struct{})

	watcherWg.Add(1)
	go func() {
		defer watcherWg.Done()
		defer func() {
			if err := watcher.Close(); err != nil {
				slog.Warn("Failed to close file watcher", "error", err)
			}
		}()

		var debounceTimer *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Chmod != 0 {
					continue
				}

				if debounceTimer != nil {
					debounceTimer.Reset(debounceConfig)
				} else {
					debounceTimer = time.AfterFunc(debounceConfig, func() {
						select {
						case reloadChan <- struct{}{}:
						default:
						}
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()
}

func stopWatcher() {
	if watcher != nil {
		if err := watcher.Close(); err != nil {
			slog.Warn("Failed to close file watcher", "error", err)
		}
	}
	watcherWg.Wait()
}
