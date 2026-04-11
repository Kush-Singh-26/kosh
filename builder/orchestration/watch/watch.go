package watch

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
)

const (
	buildQueueBuffer     = 10
	searchQueueBuffer    = 1
	debounceDelay        = 100 * time.Millisecond
	searchDelay          = 500 * time.Millisecond
	searchQuickDelay     = 100 * time.Millisecond
	searchQuickThreshold = 2 * time.Second
)

// ChangeType describes the category of a filesystem change.
type ChangeType int

const (
	// ChangeTypeContent indicates a content file change.
	ChangeTypeContent ChangeType = iota
	// ChangeTypeAsset indicates an asset file change.
	ChangeTypeAsset
	// ChangeTypeOther indicates a non-content, non-asset change.
	ChangeTypeOther
	// ChangeTypeDelete indicates a deletion event.
	ChangeTypeDelete
)

// String returns the display label for a ChangeType.
func (changeType ChangeType) String() string {
	switch changeType {
	case ChangeTypeContent:
		return "content"
	case ChangeTypeAsset:
		return "asset"
	case ChangeTypeOther:
		return "other"
	case ChangeTypeDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// ChangeEvent represents a classified filesystem change.
type ChangeEvent struct {
	Path    string
	Op      fsnotify.Op
	Type    ChangeType
	RelPath string
	Version string
}

// SearchRegenerationCallback is invoked to rebuild the search index.
type SearchRegenerationCallback func(workingContext context.Context)

// CoordinatorDependencies bundles dependencies for the watch coordinator.
type CoordinatorDependencies struct {
	Cfg           *config.Config
	BuildMu       *sync.Mutex // guards build execution in watch mode
	Cache         post.Cache
	OnChange      func(ChangeEvent)
	OnSearchRegen SearchRegenerationCallback
}

// Coordinator manages debounced change handling during watch mode.
type Coordinator struct {
	config     *config.Config
	buildMu    *sync.Mutex // guards build execution in watch mode
	cache      post.Cache
	onChange   func(ChangeEvent)
	onSearch   SearchRegenerationCallback
	buildQueue chan BuildRequest
	searchChan chan struct{}

	lastSearchReg time.Time
	closeOnce     sync.Once
	closed        chan struct{}
}

// BuildRequest groups paths for a debounced build trigger.
type BuildRequest struct {
	Paths []string
	Op    fsnotify.Op
}

// New constructs a new watch Coordinator.
func New(deps CoordinatorDependencies) *Coordinator {
	return &Coordinator{
		config:     deps.Cfg,
		buildMu:    deps.BuildMu,
		cache:      deps.Cache,
		onChange:   deps.OnChange,
		onSearch:   deps.OnSearchRegen,
		buildQueue: make(chan BuildRequest, buildQueueBuffer),
		searchChan: make(chan struct{}, searchQueueBuffer),
		closed:     make(chan struct{}),
	}
}

// Start begins processing of build and search queues.
func (coordinatorInstance *Coordinator) Start() {
	async.FireAndForget(context.Background(), slog.Default(), "watch build queue", func() error {
		coordinatorInstance.processBuildQueue()
		return nil
	})
	async.FireAndForget(context.Background(), slog.Default(), "watch search queue", func() error {
		coordinatorInstance.processSearchQueue()
		return nil
	})
}

// Close shuts down the coordinator and its queues.
func (coordinatorInstance *Coordinator) Close() {
	coordinatorInstance.closeOnce.Do(func() {
		close(coordinatorInstance.closed)
		close(coordinatorInstance.buildQueue)
		close(coordinatorInstance.searchChan)
	})
}

// EnqueueChange enqueues a path change for debounced processing.
func (coordinatorInstance *Coordinator) EnqueueChange(path string, op fsnotify.Op) {
	select {
	case coordinatorInstance.buildQueue <- BuildRequest{Paths: []string{path}, Op: op}:
	default:
	}
}

// TriggerSearchRegeneration enqueues a search regeneration request.
func (coordinatorInstance *Coordinator) TriggerSearchRegeneration() {
	select {
	case coordinatorInstance.searchChan <- struct{}{}:
	default:
	}
}

// NormalizeWatchPath normalizes a watch path relative to the working directory.
func (coordinatorInstance *Coordinator) NormalizeWatchPath(path string) string {
	workingDir, _ := os.Getwd()
	return fspkg.NormalizeWatchPath(path, workingDir)
}

// NormalizeAbsoluteWatchPath normalizes a path to an absolute, stable form.
func (coordinatorInstance *Coordinator) NormalizeAbsoluteWatchPath(path string) string {
	if absolutePath, absoluteError := fspkg.AbsNormalizePath(path); absoluteError == nil {
		return absolutePath
	}
	return fspkg.NormalizePath(path)
}

// IsContentPath reports whether a path is within the content directory.
func (coordinatorInstance *Coordinator) IsContentPath(path string) bool {
	path = coordinatorInstance.NormalizeAbsoluteWatchPath(path)
	contentDir := coordinatorInstance.NormalizeAbsoluteWatchPath(coordinatorInstance.config.ContentDir)
	return fspkg.IsPathInOrSame(path, contentDir)
}

// IsAssetPath reports whether a path is within asset directories.
func (coordinatorInstance *Coordinator) IsAssetPath(path string) bool {
	path = coordinatorInstance.NormalizeAbsoluteWatchPath(path)
	staticDir := coordinatorInstance.NormalizeAbsoluteWatchPath(coordinatorInstance.config.StaticDir)

	siteStaticDir := "static"
	if coordinatorInstance.config.SiteRoot != "" {
		siteStaticDir = filepath.Join(coordinatorInstance.config.SiteRoot, "static")
	}
	siteStaticDir = coordinatorInstance.NormalizeAbsoluteWatchPath(siteStaticDir)

	assetsDir := "assets"
	if coordinatorInstance.config.SiteRoot != "" {
		assetsDir = filepath.Join(coordinatorInstance.config.SiteRoot, "assets")
	}
	assetsDir = coordinatorInstance.NormalizeAbsoluteWatchPath(assetsDir)

	return fspkg.IsPathInOrSame(path, staticDir) || fspkg.IsPathInOrSame(path, siteStaticDir) || fspkg.IsPathInOrSame(path, assetsDir)
}

// IsSearchSourcePath reports whether a path affects search source files.
func IsSearchSourcePath(path string) bool {
	normalizedPath := fspkg.NormalizePath(path)
	return strings.HasPrefix(normalizedPath, "cmd/search/") || strings.HasPrefix(normalizedPath, "builder/search/") || strings.HasPrefix(normalizedPath, "builder/models/")
}

// InvalidateForTemplate returns paths to invalidate for a template change.
func (coordinatorInstance *Coordinator) InvalidateForTemplate(templatePath string) []string {
	normalizedTemplatePath := fspkg.NormalizePath(templatePath)
	templateDir := fspkg.NormalizePath(coordinatorInstance.config.TemplateDir)
	staticDir := fspkg.NormalizePath(coordinatorInstance.config.StaticDir)
	if strings.HasPrefix(normalizedTemplatePath, templateDir) {
		relativeTemplate, _ := fspkg.SafeRel(coordinatorInstance.config.TemplateDir, templatePath)
		relativeTemplate = fspkg.NormalizePath(relativeTemplate)

		if relativeTemplate == "layout.html" {
			return nil
		}

		if coordinatorInstance.cache != nil {
			postIdentifiers, cacheError := coordinatorInstance.cache.GetPostsByTemplate(relativeTemplate)
			if cacheError == nil && len(postIdentifiers) > 0 {
				posts, postsError := coordinatorInstance.cache.GetPostsByIDs(postIdentifiers)
				if postsError == nil && len(posts) > 0 {
					paths := make([]string, 0, len(posts))
					for _, postMetadata := range posts {
						paths = append(paths, postMetadata.Path)
					}
					return paths
				}
			}
		}
		return []string{}
	}
	if strings.HasPrefix(normalizedTemplatePath, staticDir) {
		return nil
	}

	switch normalizedTemplatePath {
	case "kosh.yaml":
		return nil
	case "builder/generators/pwa.go":
		return []string{}
	default:
		return nil
	}
}

// ClassifyChange classifies a filesystem change into a ChangeEvent.
func (coordinatorInstance *Coordinator) ClassifyChange(path string, op fsnotify.Op) ChangeEvent {
	event := ChangeEvent{Path: path, Op: op}

	if op&fsnotify.Remove != 0 {
		event.Type = ChangeTypeDelete
		return event
	}

	path = coordinatorInstance.NormalizeAbsoluteWatchPath(path)
	extension := strings.ToLower(filepath.Ext(path))

	if extension == ".md" && coordinatorInstance.IsContentPath(path) {
		event.Type = ChangeTypeContent
		return event
	}

	if (extension == ".css" || extension == ".js") && coordinatorInstance.IsAssetPath(path) {
		event.Type = ChangeTypeAsset
		return event
	}

	event.Type = ChangeTypeOther
	return event
}

func (coordinatorInstance *Coordinator) processBuildQueue() {
	var mergedPaths map[string]fsnotify.Op
	debounceTimer := time.NewTimer(debounceDelay)
	defer debounceTimer.Stop()

	for {
		select {
		case <-coordinatorInstance.closed:
			if len(mergedPaths) > 0 {
				coordinatorInstance.buildMu.Lock()
				for path, op := range mergedPaths {
					coordinatorInstance.dispatchChange(path, op)
				}
				coordinatorInstance.buildMu.Unlock()
			}
			return

		case request, isOk := <-coordinatorInstance.buildQueue:
			if !isOk {
				if len(mergedPaths) > 0 {
					coordinatorInstance.buildMu.Lock()
					for path, op := range mergedPaths {
						coordinatorInstance.dispatchChange(path, op)
					}
					coordinatorInstance.buildMu.Unlock()
				}
				return
			}
			if mergedPaths == nil {
				mergedPaths = make(map[string]fsnotify.Op)
			}
			for _, path := range request.Paths {
				mergedPaths[path] = request.Op
			}
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(debounceDelay)

		case <-debounceTimer.C:
			if len(mergedPaths) > 0 {
				for path, op := range mergedPaths {
					coordinatorInstance.dispatchChange(path, op)
				}
				mergedPaths = nil
			}
		}
	}
}

func (coordinatorInstance *Coordinator) dispatchChange(path string, op fsnotify.Op) {
	if coordinatorInstance.onChange == nil {
		return
	}
	coordinatorInstance.onChange(ChangeEvent{
		Path: path,
		Op:   op,
		Type: coordinatorInstance.classify(path, op),
	})
}

func (coordinatorInstance *Coordinator) classify(path string, op fsnotify.Op) ChangeType {
	if op&fsnotify.Remove != 0 {
		return ChangeTypeDelete
	}
	path = coordinatorInstance.NormalizeAbsoluteWatchPath(path)
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".md" && coordinatorInstance.IsContentPath(path) {
		return ChangeTypeContent
	}
	if (extension == ".css" || extension == ".js") && coordinatorInstance.IsAssetPath(path) {
		return ChangeTypeAsset
	}
	return ChangeTypeOther
}

func (coordinatorInstance *Coordinator) processSearchQueue() {
	var pending bool
	var timer *time.Timer
	var timerRunning bool

	for {
		select {
		case <-coordinatorInstance.closed:
			if timer != nil && timerRunning {
				timer.Stop()
			}
			return
		case <-coordinatorInstance.searchChan:
			pending = true
			if !timerRunning {
				delay := searchDelay
				if time.Since(coordinatorInstance.lastSearchReg) > searchQuickThreshold {
					delay = searchQuickDelay
				}
				timer = time.AfterFunc(delay, func() {
					if pending {
						pending = false
						async.FireAndForget(context.Background(), slog.Default(), "watch search regen", func() error {
							coordinatorInstance.buildMu.Lock()
							defer coordinatorInstance.buildMu.Unlock()
							coordinatorInstance.lastSearchReg = time.Now()
							if coordinatorInstance.onSearch != nil {
								coordinatorInstance.onSearch(context.Background())
							}
							return nil
						})
					}
				})
				timerRunning = true
			}
		}
	}
}
