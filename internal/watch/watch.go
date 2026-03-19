package watch

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/fsnotify/fsnotify"
)

// Event is a wrapper around fsnotify.Event
type Event struct {
	Name string
	Op   fsnotify.Op
}

// Watcher handles filesystem events and triggers builds
type Watcher struct {
	watcher  *fsnotify.Watcher
	Dirs     []string
	OnEvent  func(Event)
	timerMu  sync.Mutex
	timer    *time.Timer
	duration time.Duration
}

// New creates a new watcher for the specified directories
func New(dirs []string, onEvent func(Event)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		watcher:  w,
		Dirs:     dirs,
		OnEvent:  onEvent,
		duration: 50 * time.Millisecond, // 50ms debounce for fast dev response
	}, nil
}

// resetTimer safely stops the existing timer and creates a new one.
// This method is thread-safe and properly drains the timer channel if needed.
func (w *Watcher) resetTimer(pendingMu *sync.Mutex, pendingEvents *map[string]fsnotify.Op) {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()

	if w.timer != nil {
		if !w.timer.Stop() {
			// Timer already fired, drain the channel to prevent goroutine leak
			select {
			case <-w.timer.C:
			default:
			}
		}
	}

	w.timer = time.AfterFunc(w.duration, func() {
		pendingMu.Lock()
		eventsToProcess := *pendingEvents
		*pendingEvents = make(map[string]fsnotify.Op)
		pendingMu.Unlock()

		for name, op := range eventsToProcess {
			w.OnEvent(Event{Name: name, Op: op})
		}
	})
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
			slog.Error("Error walking directory", "dir", dir, "error", err)
		}
	}

	orchestration.DevLogInfo("Watch mode active. Waiting for changes...")

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

			// Safely reset the debounce timer
			w.resetTimer(&pendingMu, &pendingEvents)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Watcher error", "error", err)
		}
	}
}
