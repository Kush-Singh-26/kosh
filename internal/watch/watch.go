package watch

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event is a wrapper around fsnotify.Event
type Event struct {
	Name string
	Op   fsnotify.Op
}

// Watcher handles filesystem events and triggers builds
type Watcher struct {
	watcher *fsnotify.Watcher
	Dirs    []string
	OnEvent func(Event)
}

// New creates a new watcher for the specified directories
func New(dirs []string, onEvent func(Event)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		watcher: w,
		Dirs:    dirs,
		OnEvent: onEvent,
	}, nil
}

// Start begins watching for events
func (w *Watcher) Start() {
	defer func() { _ = w.watcher.Close() }()

	// Add directories recursively
	for _, dir := range w.Dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Skip hidden directories like .git
				if filepath.Base(path)[0] == '.' && path != "." {
					return filepath.SkipDir
				}
				return w.watcher.Add(path)
			}
			return nil
		})
		if err != nil {
			log.Printf("Error walking %s: %v", dir, err)
		}
	}

	log.Println("👀 Watch mode active. Waiting for changes...")

	// Debounce timer
	var timer *time.Timer
	const debounceDuration = 100 * time.Millisecond

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Ignore chmod and other meta events
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}

			// Handle new directories
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					_ = w.watcher.Add(event.Name)
				}
			}

			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceDuration, func() {
				w.OnEvent(Event{Name: event.Name, Op: event.Op})
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}
