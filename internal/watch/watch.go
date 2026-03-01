package watch

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
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
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
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

	// Debounce timer - reduced to 50ms for faster response in dev mode
	var timer *time.Timer
	const debounceDuration = 50 * time.Millisecond

	var pendingMu sync.Mutex
	pendingEvents := make(map[string]fsnotify.Op)

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

			pendingMu.Lock()
			// Combine operations if same file modified multiple times
			existingOp, exists := pendingEvents[event.Name]
			if exists {
				pendingEvents[event.Name] = existingOp | event.Op
			} else {
				pendingEvents[event.Name] = event.Op
			}
			pendingMu.Unlock()

			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceDuration, func() {
				pendingMu.Lock()
				eventsToProcess := pendingEvents
				pendingEvents = make(map[string]fsnotify.Op)
				pendingMu.Unlock()

				for name, op := range eventsToProcess {
					w.OnEvent(Event{Name: name, Op: op})
				}
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}
