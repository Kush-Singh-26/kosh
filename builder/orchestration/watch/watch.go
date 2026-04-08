package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
)

// ChangeType describes the category of a filesystem change.
type ChangeType string

const (
	// ChangeTypeContent indicates a content file change.
	ChangeTypeContent ChangeType = "content"
	// ChangeTypeAsset indicates an asset file change.
	ChangeTypeAsset ChangeType = "asset"
	// ChangeTypeOther indicates a non-content, non-asset change.
	ChangeTypeOther ChangeType = "other"
	// ChangeTypeDelete indicates a deletion event.
	ChangeTypeDelete ChangeType = "delete"
)

// ChangeEvent represents a classified filesystem change.
type ChangeEvent struct {
	Path    string
	Op      fsnotify.Op
	Type    ChangeType
	RelPath string
	Version string
}

// SearchRegenerationCallback is invoked to rebuild the search index.
type SearchRegenerationCallback func(ctx context.Context)

// CoordinatorDependencies bundles dependencies for the watch coordinator.
type CoordinatorDependencies struct {
	Cfg           *config.Config
	BuildMu       *sync.Mutex
	Cache         post.Cache
	OnChange      func(ChangeEvent)
	OnSearchRegen SearchRegenerationCallback
}

// Coordinator manages debounced change handling during watch mode.
type Coordinator struct {
	cfg        *config.Config
	buildMu    *sync.Mutex
	cache      post.Cache
	onChange   func(ChangeEvent)
	onSearch   SearchRegenerationCallback
	buildQueue chan BuildRequest
	searchCh   chan struct{}

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
		cfg:        deps.Cfg,
		buildMu:    deps.BuildMu,
		cache:      deps.Cache,
		onChange:   deps.OnChange,
		onSearch:   deps.OnSearchRegen,
		buildQueue: make(chan BuildRequest, 10),
		searchCh:   make(chan struct{}, 1),
		closed:     make(chan struct{}),
	}
}

// Start begins processing of build and search queues.
func (c *Coordinator) Start() {
	go c.processBuildQueue()
	go c.processSearchQueue()
}

// Close shuts down the coordinator and its queues.
func (c *Coordinator) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		close(c.buildQueue)
		close(c.searchCh)
	})
}

// EnqueueChange enqueues a path change for debounced processing.
func (c *Coordinator) EnqueueChange(path string, op fsnotify.Op) {
	select {
	case c.buildQueue <- BuildRequest{Paths: []string{path}, Op: op}:
	default:
	}
}

// TriggerSearchRegeneration enqueues a search regeneration request.
func (c *Coordinator) TriggerSearchRegeneration() {
	select {
	case c.searchCh <- struct{}{}:
	default:
	}
}

// NormalizeWatchPath normalizes a watch path relative to the working directory.
func (c *Coordinator) NormalizeWatchPath(path string) string {
	wd, _ := os.Getwd()
	return fspkg.NormalizeWatchPath(path, wd)
}

// NormalizeAbsoluteWatchPath normalizes a path to an absolute, stable form.
func (c *Coordinator) NormalizeAbsoluteWatchPath(path string) string {
	if abs, err := fspkg.AbsNormalizePath(path); err == nil {
		return abs
	}
	return fspkg.NormalizePath(path)
}

// IsContentPath reports whether a path is within the content directory.
func (c *Coordinator) IsContentPath(path string) bool {
	path = c.NormalizeAbsoluteWatchPath(path)
	contentDir := c.NormalizeAbsoluteWatchPath(c.cfg.ContentDir)
	return fspkg.IsPathInOrSame(path, contentDir)
}

// IsAssetPath reports whether a path is within asset directories.
func (c *Coordinator) IsAssetPath(path string) bool {
	path = c.NormalizeAbsoluteWatchPath(path)
	staticDir := c.NormalizeAbsoluteWatchPath(c.cfg.StaticDir)
	siteStaticDir := "static"
	if c.cfg.SiteRoot != "" {
		siteStaticDir = filepath.Join(c.cfg.SiteRoot, "static")
	}
	siteStaticDir = c.NormalizeAbsoluteWatchPath(siteStaticDir)
	return fspkg.IsPathInOrSame(path, staticDir) || fspkg.IsPathInOrSame(path, siteStaticDir)
}

// IsSearchSourcePath reports whether a path affects search source files.
func IsSearchSourcePath(path string) bool {
	path = fspkg.NormalizePath(path)
	return strings.HasPrefix(path, "cmd/search/") || strings.HasPrefix(path, "builder/search/") || strings.HasPrefix(path, "builder/models/")
}

// InvalidateForTemplate returns paths to invalidate for a template change.
func (c *Coordinator) InvalidateForTemplate(templatePath string) []string {
	tp := fspkg.NormalizePath(templatePath)
	templateDir := fspkg.NormalizePath(c.cfg.TemplateDir)
	staticDir := fspkg.NormalizePath(c.cfg.StaticDir)
	if strings.HasPrefix(tp, templateDir) {
		relTmpl, _ := fspkg.SafeRel(c.cfg.TemplateDir, templatePath)
		relTmpl = fspkg.NormalizePath(relTmpl)

		if relTmpl == "layout.html" {
			return nil
		}

		if c.cache != nil {
			ids, err := c.cache.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				posts, err := c.cache.GetPostsByIDs(ids)
				if err == nil && len(posts) > 0 {
					paths := make([]string, 0, len(posts))
					for _, post := range posts {
						paths = append(paths, post.Path)
					}
					return paths
				}
			}
		}
		return []string{}
	}
	if strings.HasPrefix(tp, staticDir) {
		return nil
	}

	switch tp {
	case "kosh.yaml":
		return nil
	case "builder/generators/pwa.go":
		return []string{}
	default:
		return nil
	}
}

// ClassifyChange classifies a filesystem change into a ChangeEvent.
func (c *Coordinator) ClassifyChange(path string, op fsnotify.Op) ChangeEvent {
	evt := ChangeEvent{Path: path, Op: op}

	if op&fsnotify.Remove != 0 {
		evt.Type = ChangeTypeDelete
		return evt
	}

	path = c.NormalizeAbsoluteWatchPath(path)
	ext := strings.ToLower(filepath.Ext(path))

	if ext == ".md" && c.IsContentPath(path) {
		evt.Type = ChangeTypeContent
		return evt
	}

	if (ext == ".css" || ext == ".js") && c.IsAssetPath(path) {
		evt.Type = ChangeTypeAsset
		return evt
	}

	evt.Type = ChangeTypeOther
	return evt
}

func (c *Coordinator) processBuildQueue() {
	var mergedPaths map[string]fsnotify.Op
	debounce := time.NewTimer(100 * time.Millisecond)
	defer debounce.Stop()

	for {
		select {
		case <-c.closed:
			if len(mergedPaths) > 0 {
				c.buildMu.Lock()
				for path, op := range mergedPaths {
					c.dispatchChange(path, op)
				}
				c.buildMu.Unlock()
			}
			return

		case req, ok := <-c.buildQueue:
			if !ok {
				if len(mergedPaths) > 0 {
					c.buildMu.Lock()
					for path, op := range mergedPaths {
						c.dispatchChange(path, op)
					}
					c.buildMu.Unlock()
				}
				return
			}
			if mergedPaths == nil {
				mergedPaths = make(map[string]fsnotify.Op)
			}
			for _, path := range req.Paths {
				mergedPaths[path] = req.Op
			}
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(100 * time.Millisecond)

		case <-debounce.C:
			if len(mergedPaths) > 0 {
				for path, op := range mergedPaths {
					c.dispatchChange(path, op)
				}
				mergedPaths = nil
			}
		}
	}
}

func (c *Coordinator) dispatchChange(path string, op fsnotify.Op) {
	if c.onChange == nil {
		return
	}
	c.onChange(ChangeEvent{
		Path: path,
		Op:   op,
		Type: c.classify(path, op),
	})
}

func (c *Coordinator) classify(path string, op fsnotify.Op) ChangeType {
	if op&fsnotify.Remove != 0 {
		return ChangeTypeDelete
	}
	path = c.NormalizeAbsoluteWatchPath(path)
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" && c.IsContentPath(path) {
		return ChangeTypeContent
	}
	if (ext == ".css" || ext == ".js") && c.IsAssetPath(path) {
		return ChangeTypeAsset
	}
	return ChangeTypeOther
}

func (c *Coordinator) processSearchQueue() {
	var pending bool
	var timer *time.Timer
	var timerRunning bool

	for {
		select {
		case <-c.closed:
			if timer != nil && timerRunning {
				timer.Stop()
			}
			return
		case <-c.searchCh:
			pending = true
			if !timerRunning {
				delay := 500 * time.Millisecond
				if time.Since(c.lastSearchReg) > 2*time.Second {
					delay = 100 * time.Millisecond
				}
				timer = time.AfterFunc(delay, func() {
					if pending {
						pending = false
						go func() {
							c.buildMu.Lock()
							defer c.buildMu.Unlock()
							c.lastSearchReg = time.Now()
							if c.onSearch != nil {
								c.onSearch(context.Background())
							}
						}()
					}
				})
				timerRunning = true
			}
		}
	}
}
