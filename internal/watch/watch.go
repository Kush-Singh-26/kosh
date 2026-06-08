package watch

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/orchestration"
)

const watchDebounceDuration = 50 * time.Millisecond

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
	timerMu  sync.Mutex // protects timer
	timer    *time.Timer
	duration time.Duration
}

// New creates a new watcher for the specified directories
func New(dirs []string, onEvent func(Event)) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		watcher:  fsWatcher,
		Dirs:     dirs,
		OnEvent:  onEvent,
		duration: watchDebounceDuration, // 50ms debounce for fast dev response
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

// Close stops watching for changes
func (w *Watcher) Close() error {
	w.timerMu.Lock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timerMu.Unlock()
	return w.watcher.Close()
}

// Start begins watching for events
func (w *Watcher) Start() {
	w.addWatchDirs()
	orchestration.DevLogInfo("Watch mode active. Waiting for changes...")
	w.handleWatchEvents()
}

func (w *Watcher) addWatchDirs() {
	for _, dir := range w.Dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				// Skip hidden directories like .git
				if filepath.Base(path)[0] == '.' && path != "." {
					return filepath.SkipDir
				}
				return w.watcher.Add(path)
			}
			// If the explicitly provided path is a file, watch it directly
			if path == dir {
				return w.watcher.Add(path)
			}
			return nil
		})
		if err != nil {
			slog.Error("Error walking directory", "dir", dir, "error", err)
		}
	}
}

func (w *Watcher) handleWatchEvents() {
	var pendingMu sync.Mutex
	pendingEvents := make(map[string]fsnotify.Op)

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					_ = w.watcher.Add(event.Name)
				}
			}

			pendingMu.Lock()
			pendingEvents[event.Name] |= event.Op
			pendingMu.Unlock()

			w.resetTimer(&pendingMu, &pendingEvents)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Watcher error", "error", err)
		}
	}
}
