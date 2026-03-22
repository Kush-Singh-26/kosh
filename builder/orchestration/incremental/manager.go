package incremental

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/wasm"
)

type PostChangeType int

const (
	PostChangeNone PostChangeType = iota
	PostChangeNew
	PostChangeFrontmatter
	PostChangeBody
)

type SiteBuilder interface {
	Build(ctx context.Context) error
	BuildAssetOnly(ctx context.Context) error
	SaveCaches()
	RefreshBuildSession()
	Commit(ctx context.Context) error
	GetWatch() WatchCoordinator
	GetRender() render.Service
	GetPost() post.Service
}

type SearchManager interface {
	UpdateIndexedPostCache(relPath string, parseRes *post.ParsedMarkdownResult)
	PruneDeletedPost(relPath string)
}

type WatchCoordinator interface {
	ClassifyChange(path string, op fsnotify.Op) watch.ChangeEvent
	TriggerSearchRegeneration()
}

type IncrementalDependencies struct {
	Cache    svcCache.Service
	Post     post.Service
	Render   render.Service
	Wasm     wasm.Service
	Diagrams *cache.DiagramCacheAdapter
}

type ManagerDependencies struct {
	Cfg            *config.Config
	Logger         *slog.Logger
	SourceFs       afero.Fs
	Deps           IncrementalDependencies
	Builder        SiteBuilder
	Search         SearchManager
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
}

type Manager struct {
	cfg            *config.Config
	logger         *slog.Logger
	sourceFs       afero.Fs
	deps           IncrementalDependencies
	builder        SiteBuilder
	search         SearchManager
	mdPool         *sync.Pool
	nativeRenderer *native.Renderer
}

func NewManager(deps ManagerDependencies) *Manager {
	return &Manager{
		cfg:            deps.Cfg,
		logger:         deps.Logger,
		sourceFs:       deps.SourceFs,
		deps:           deps.Deps,
		builder:        deps.Builder,
		search:         deps.Search,
		mdPool:         deps.MdPool,
		nativeRenderer: deps.NativeRenderer,
	}
}

func (m *Manager) BuildSingleFileChange(ctx context.Context, path string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	m.logger.Info("Processing queued change", "path", filepath.Base(path), "op", op.String())

	m.cfg.BuildVersion = time.Now().UnixNano()
	m.builder.GetRender().ReloadTemplates()
	m.builder.GetRender().SetAssetsGate(nil)
	m.builder.GetPost().SetAssetsGate(nil)

	watchCoordinator := m.builder.GetWatch()
	if watchCoordinator != nil {
		evt := watchCoordinator.ClassifyChange(path, op)
		switch evt.Type {
		case watch.ChangeTypeContent:
			m.HandleMarkdownChange(ctx, path)
			return
		case watch.ChangeTypeAsset:
			m.HandleAssetChange(ctx, path)
			return
		case watch.ChangeTypeDelete:
			m.HandleMarkdownChange(ctx, path)
			return
		}
	}

	m.HandleOtherChange(ctx, path)
}

func (m *Manager) HandleMarkdownChange(ctx context.Context, path string) {
	m.builder.RefreshBuildSession()
	m.BuildSinglePost(ctx, path)
	if err := m.builder.Commit(ctx); err != nil {
		m.logger.Error("Sync/Commit failed", "error", err)
		m.deletePostFromCache(path)
		return
	}
	m.deps.Render.ClearRenderedFiles()
}

// HandleAssetChange handles CSS/JS changes
func (m *Manager) HandleAssetChange(ctx context.Context, path string) {
	m.logger.Info("CSS/JS changed, running full rebuild...")
	if err := m.builder.BuildAssetOnly(ctx); err != nil {
		m.logger.Error("Build failed", "error", err)
		return
	}
	m.builder.SaveCaches()
}

// HandleOtherChange handles other file changes
func (m *Manager) HandleOtherChange(ctx context.Context, path string) {
	if err := m.builder.Build(ctx); err != nil {
		m.logger.Error("Build failed", "error", err)
		return
	}
	m.builder.SaveCaches()
}

// BuildSinglePost rebuilds only the changed post with smart change detection
func (m *Manager) BuildSinglePost(ctx context.Context, path string) {
	source, err := afero.ReadFile(m.sourceFs, path)
	if err != nil {
		m.logger.Error("Error reading file", "path", path, "error", err)
		if buildErr := m.triggerFullBuild(ctx); buildErr != nil {
			m.logger.Error("Full build failed", "error", buildErr)
		}
		return
	}

	relPath, version, htmlRelPath, cleanHtmlRelPath, err := m.ResolveContentPaths(path)
	if err != nil {
		m.logger.Error("Path resolution failed", "error", err)
		return
	}

	m.logger.Debug("incremental content path resolved", "path", path, "relative", relPath, "version", version)

	newFrontmatterHash, newBodyHash := m.ComputePostHashes(source)
	changeType := m.DeterminePostChange(relPath, newFrontmatterHash, newBodyHash)

	switch changeType {
	case PostChangeNew, PostChangeFrontmatter:
		m.logger.Info("New or frontmatter change detected, running full build...")
		if err := m.triggerFullBuild(ctx); err != nil {
			m.logger.Error("Build failed", "error", err)
		}
		return

	case PostChangeBody:
		m.logger.Info("Content change detected, rebuilding single post...")

		parseRes, err := post.ParseMarkdown(
			post.ParseConfig{
				Source:           source,
				Path:             path,
				Version:          version,
				CleanHtmlRelPath: cleanHtmlRelPath,
				HtmlRelPath:      htmlRelPath,
			},
			post.ParseContext{
				MdPool:         m.mdPool,
				Cfg:            m.cfg,
				NativeRenderer: m.nativeRenderer,
				DiagramAdapter: m.deps.Diagrams,
				MathBatchSize:  post.DefaultMathBatchSize,
			},
		)

		if err != nil {
			m.logger.Error("Error parsing markdown", "path", path, "error", err)
			return
		}

		if err := m.deps.Post.ProcessSingleWithResult(ctx, path, source, parseRes); err != nil {
			m.logger.Error("Failed to process single post", "error", err)
			if err := m.triggerFullBuild(ctx); err != nil {
				m.logger.Error("Build failed", "error", err)
				return
			}
		} else {
			if m.search != nil {
				m.search.UpdateIndexedPostCache(relPath, parseRes)
			}
		}
		m.builder.SaveCaches()

		if watch := m.builder.GetWatch(); watch != nil {
			watch.TriggerSearchRegeneration()
		}

	case PostChangeNone:
		m.logger.Info("No changes detected, skipping...")
	}
}

func (m *Manager) deletePostFromCache(path string) {
	relPath, err := fspkg.SafeRel(m.cfg.ContentDir, path)
	if err != nil {
		m.logger.Error("Failed to get relative path for deletion", "path", path, "error", err)
		return
	}

	if m.deps.Cache == nil {
		return
	}

	postID := cache.GeneratePostID("", relPath)
	if err := m.deps.Cache.DeletePost(postID); err != nil {
		m.logger.Error("Failed to delete post from cache", "postID", postID, "error", err)
		return
	}

	if m.search != nil {
		m.search.PruneDeletedPost(relPath)
	}

	m.logger.Info("Removed deleted post from cache", "path", relPath)
}

func (m *Manager) triggerFullBuild(ctx context.Context) error {
	if err := m.builder.Build(ctx); err != nil {
		return err
	}
	m.builder.SaveCaches()
	return nil
}

// ResolveContentPaths resolves various path formats for incremental builds.
func (m *Manager) ResolveContentPaths(path string) (relPath, version, htmlRelPath, cleanHtmlRelPath string, err error) {
	contentRoot, err := filepath.Abs(m.cfg.ContentDir)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve content directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve changed path: %w", err)
	}
	relPath, err = filepath.Rel(contentRoot, absPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to compute content-relative path: %w", err)
	}
	relPath = fspkg.NormalizePath(relPath)
	version, relPath = navigation.GetVersionFromPath(fspkg.NormalizePath(filepath.Join("content", relPath)))

	htmlRelPath = strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	cleanHtmlRelPath = htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
	}
	return relPath, version, htmlRelPath, cleanHtmlRelPath, nil
}

// ComputePostHashes calculates frontmatter and body hashes for change detection.
func (m *Manager) ComputePostHashes(source []byte) (frontmatterHash, bodyHash string) {
	frontmatterHash, _ = hashing.GetFrontmatterHashFromSource(source)
	bodyHash = hashing.GetBodyHash(source)
	return frontmatterHash, bodyHash
}

// DeterminePostChange compares current hashes with cache to determine change type.
func (m *Manager) DeterminePostChange(relPath, newFrontmatterHash, newBodyHash string) PostChangeType {
	if m.deps.Cache == nil {
		return PostChangeNew
	}
	meta, err := m.deps.Cache.GetPostByPath(relPath)
	if err != nil || meta == nil {
		return PostChangeNew
	}

	if meta.ContentHash != newFrontmatterHash {
		return PostChangeFrontmatter
	}
	if meta.BodyHash != newBodyHash || meta.BodyHash == "" {
		return PostChangeBody
	}
	return PostChangeNone
}
