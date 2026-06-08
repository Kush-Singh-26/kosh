package incremental

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// PostChangeType describes the kind of change detected for a Content.
type PostChangeType int

const (
	// PostChangeNone indicates no meaningful change.
	PostChangeNone PostChangeType = iota
	// PostChangeNew indicates a new Content.
	PostChangeNew
	// PostChangeFrontmatter indicates frontmatter-only changes.
	PostChangeFrontmatter
	// PostChangeBody indicates body content changes.
	PostChangeBody
)

// SiteBuilder defines the build operations used by the incremental manager.
type SiteBuilder interface {
	Build(ctx context.Context) error
	BuildLocked(ctx context.Context) error
	BuildAssetOnly(ctx context.Context) error
	BuildAssetOnlyWithOptions(ctx context.Context, forceImages bool) error
	ReloadConfig(ctx context.Context) error
	SaveCaches()
	RefreshBuildSession()
	Commit(ctx context.Context) error
	GetWatch() WatchCoordinator
	GetRender() render.Service
	GetContent() content.Service
	LockBuild()
	UnlockBuild()
	RenderSiteWide(ctx context.Context, cb *content.Context) error
}

// SearchManager provides hooks for search index updates during incremental builds.
type SearchManager interface {
	UpdateIndexedContentCache(relPath string, parseRes *content.ParsedMarkdownResult)
	PruneDeletedItem(relPath string)
}

// WatchCoordinator provides change classification during watch mode.
type WatchCoordinator interface {
	ClassifyChange(path string, op fsnotify.Op) watch.ChangeEvent
	TriggerSearchRegeneration()
}

// Dependencies groups optional services used by the incremental manager.
type Dependencies struct {
	Cache    svcCache.Service
	Content  content.Service
	Render   render.Service
	Diagrams *cache.DiagramCacheAdapter
}

// ManagerDependencies holds dependencies for creating an incremental Manager.
type ManagerDependencies struct {
	Cfg            *config.Config
	Logger         *slog.Logger
	SourceFs       afero.Fs
	Deps           Dependencies
	Builder        SiteBuilder
	Search         SearchManager
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
}

// Manager coordinates incremental build behavior for file changes.
type Manager struct {
	cfg            *config.Config
	logger         *slog.Logger
	sourceFs       afero.Fs
	deps           Dependencies
	builder        SiteBuilder
	search         SearchManager
	mdPool         *sync.Pool
	nativeRenderer *native.Renderer
}

// NewManager constructs a new incremental Manager.
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

// ReconfigureWithLogger updates the logger for the manager.
func (m *Manager) ReconfigureWithLogger(l *slog.Logger) {
	m.logger = l
}

// BuildSingleFileChange handles a single filesystem change event.
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
	m.builder.GetContent().SetAssetsGate(nil)

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
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".md":
				m.HandleDeleteChange(ctx, path)
			case ".png", ".jpg", ".jpeg", ".css", ".js", ".svg", ".webp", ".gif":
				m.HandleAssetChange(ctx, path)
			default:
				m.HandleOtherChange(ctx, path)
			}
			return
		}
	}

	m.HandleOtherChange(ctx, path)
}

// HandleMarkdownChange handles markdown content changes.
func (m *Manager) HandleMarkdownChange(ctx context.Context, path string) {
	m.builder.LockBuild()
	defer m.builder.UnlockBuild()

	m.BuildSingleItem(ctx, path)
}

// HandleAssetChange handles CSS/JS changes
func (m *Manager) HandleAssetChange(ctx context.Context, path string) {
	m.builder.LockBuild()
	defer m.builder.UnlockBuild()

	ext := strings.ToLower(filepath.Ext(path))
	forceImages := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif" || ext == ".svg"

	if forceImages {
		m.logger.Info("Image in static/ changed, running asset-only rebuild with image processing...")
	} else {
		m.logger.Info("CSS/JS changed, running asset-only rebuild (skipping image processing)...")
	}

	if err := m.builder.BuildAssetOnlyWithOptions(ctx, forceImages); err != nil {
		m.logger.Error("Build failed", "error", err)
		return
	}
	m.builder.SaveCaches()
}

// HandleOtherChange handles other file changes
func (m *Manager) HandleOtherChange(ctx context.Context, path string) {
	m.builder.LockBuild()
	defer m.builder.UnlockBuild()

	base := filepath.Base(path)
	if base == "kosh.yaml" || base == "config.yaml" {
		m.logger.Info("Configuration change detected, reloading config and forcing full rebuild...")
		if err := m.builder.ReloadConfig(ctx); err != nil {
			m.logger.Error("Failed to reload configuration", "error", err)
		}
	}

	if err := m.builder.BuildLocked(ctx); err != nil {
		m.logger.Error("Build failed", "error", err)
		return
	}
	m.builder.SaveCaches()
}

// BuildSingleItem rebuilds only the changed Content with smart change detection
func (m *Manager) BuildSingleItem(ctx context.Context, path string) {
	source, err := afero.ReadFile(m.sourceFs, path)
	if err != nil {
		m.logger.Error("Error reading file", "path", path, "error", err)
		if buildErr := m.builder.BuildLocked(ctx); buildErr != nil {
			m.logger.Error("Full build failed", "error", buildErr)
		}
		return
	}

	relPath, htmlRelPath, cleanHTMLRelPath, err := m.ResolveContentPaths(path)
	if err != nil {
		m.logger.Error("Path resolution failed", "error", err)
		return
	}

	m.logger.Debug("Incremental content path resolved", "path", path, "relative", relPath)

	// Ensure hashing matches scanner fallback behavior
	fallbackTitle := strings.TrimSuffix(filepath.Base(path), ".md")
	newFrontmatterHash, newBodyHash := m.ComputePostHashes(source, fallbackTitle)
	changeType := m.DeterminePostChange(relPath, newFrontmatterHash, newBodyHash)

	switch changeType {
	case PostChangeNew, PostChangeFrontmatter:
		m.logger.Info("New or frontmatter change detected, running incremental build...")
		if err := m.handleSinglePostBodyChange(ctx, path, source, relPath, htmlRelPath, cleanHTMLRelPath, newFrontmatterHash, newBodyHash); err != nil {
			m.logger.Error("Incremental rebuild failed", "error", err)
			if buildErr := m.builder.BuildLocked(ctx); buildErr != nil {
				m.logger.Error("Full build fallback failed", "error", buildErr)
			}
		}

	case PostChangeBody:
		if err := m.handleSinglePostBodyChange(ctx, path, source, relPath, htmlRelPath, cleanHTMLRelPath, newFrontmatterHash, newBodyHash); err != nil {
			m.logger.Error("Single post body rebuild failed", "error", err)
		}

	case PostChangeNone:
		m.logger.Info("No changes detected, skipping...")
	}
}

func (m *Manager) handleSinglePostBodyChange(ctx context.Context, path string, source []byte, relPath, htmlRelPath, cleanHTMLRelPath, knownFrontmatterHash, newBodyHash string) error {
	m.logger.Info("Content change detected, rebuilding single Content...")

	// Only refresh session and commit for body-only changes.
	// Full builds (via BuildLocked) manage their own session.
	m.builder.RefreshBuildSession()

	if m.deps.Content != nil {
		processed, err := m.deps.Content.ProcessShortcodes(source)
		if err == nil {
			source = processed
		}
	}

	preParsedMeta, bodyOffset, knownReadingTime := m.resolveIncrementalParseInputs(path, relPath, source, newBodyHash)

	parseRes, err := content.ParseMarkdown(ctx, content.ParseOptions{
		Source:               source,
		Path:                 path,
		RelPath:              relPath,
		CleanHTMLRelPath:     cleanHTMLRelPath,
		HTMLRelPath:          htmlRelPath,
		MdPool:               m.mdPool,
		Cfg:                  m.cfg,
		NativeRenderer:       m.nativeRenderer,
		DiagramAdapter:       m.deps.Diagrams,
		KnownFrontmatterHash: knownFrontmatterHash,
		KnownReadingTime:     knownReadingTime,
		BodyOffset:           bodyOffset,
		PreParsedMeta:        preParsedMeta,
	})

	if err != nil {
		m.logger.Error("Error parsing markdown", "path", path, "error", err)
		return err
	}

	if err := m.deps.Content.ProcessSingleWithResult(ctx, path, source, parseRes); err != nil {
		m.logger.Error("Failed to process single Content", "error", err)
		if err := m.builder.BuildLocked(ctx); err != nil {
			m.logger.Error("Full build failed", "error", err)
		}
		return err
	}

	m.updateSearchAndNeighbors(ctx, relPath, parseRes)

	if err := m.renderIncrementalSiteWide(ctx); err != nil {
		return err
	}

	if err := m.commitIncrementalBuild(ctx, path); err != nil {
		return err
	}

	m.builder.SaveCaches()
	m.deps.Render.ClearRenderedFiles()

	if watch := m.builder.GetWatch(); watch != nil {
		watch.TriggerSearchRegeneration()
	}

	return nil
}

func (m *Manager) updateSearchAndNeighbors(ctx context.Context, relPath string, parseRes *content.ParsedMarkdownResult) {
	if m.search != nil {
		m.search.UpdateIndexedContentCache(relPath, parseRes)
	}

	if m.deps.Content != nil {
		m.deps.Content.WaitForCacheCommit()
		// Re-render immediate neighbors to ensure their navigation links are up-to-date.
		m.rebuildNeighbors(ctx, parseRes.Item.Link)
	}
}

func (m *Manager) renderIncrementalSiteWide(ctx context.Context) error {
	metadataCtx, err := m.builder.GetContent().GetMetadataContext(ctx)
	if err != nil {
		m.logger.Error("Failed to retrieve metadata context for site-wide rendering", "error", err)
		return err
	}

	// Force the engine to execute the site-wide generators (RSS, pagination, sitemap)
	// so they immediately reflect frontmatter and file changes made during dev mode.
	metadataCtx.AnyItemChanged = true

	if err := m.builder.RenderSiteWide(ctx, metadataCtx); err != nil {
		m.logger.Error("Failed to update site-wide assets during incremental build", "error", err)
		return err
	}
	return nil
}

func (m *Manager) commitIncrementalBuild(ctx context.Context, path string) error {
	if err := m.builder.Commit(ctx); err != nil {
		m.logger.Error("Sync/Commit failed", "error", err)
		m.DeleteItemFromCache(path)
		return err
	}
	return nil
}

// DeleteItemFromCache removes an item from the cache by its path.
func (m *Manager) DeleteItemFromCache(path string) {
	relPath, err := fspkg.SafeRel(m.cfg.ContentDir, path)
	if err != nil {
		m.logger.Error("Failed to get relative path for deletion", "path", path, "error", err)
		return
	}

	if m.deps.Cache == nil {
		return
	}

	contentID := core.GenerateContentID("", relPath)
	if err := m.deps.Cache.DeleteItem(contentID); err != nil {
		m.logger.Error("Failed to delete Content from cache", "contentID", contentID, "error", err)
		return
	}

	if m.search != nil {
		m.search.PruneDeletedItem(relPath)
	}

	m.logger.Info("Removed deleted Content from cache", "path", relPath)
}

// ResolveContentPaths resolves various path formats for incremental builds.
func (m *Manager) ResolveContentPaths(path string) (string, string, string, error) {
	contentRoot, err := filepath.Abs(m.cfg.ContentDir)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to resolve content directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to resolve changed path: %w", err)
	}
	relPath, err := filepath.Rel(contentRoot, absPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to compute content-relative path: %w", err)
	}
	relPath = fspkg.NormalizePath(relPath)

	htmlRelPath := fspkg.MarkdownToHTMLPath(relPath)
	cleanHTMLRelPath := htmlRelPath
	return relPath, htmlRelPath, cleanHTMLRelPath, nil
}

// ComputePostHashes calculates frontmatter and body hashes for change detection.
func (m *Manager) ComputePostHashes(source []byte, fallbackTitle string) (frontmatterHash, bodyHash string) {
	taxonomyKeys := make([]string, 0, len(m.cfg.Taxonomies))
	for k := range m.cfg.Taxonomies {
		taxonomyKeys = append(taxonomyKeys, k)
	}
	frontmatterHash, _ = hashing.GetFrontmatterHashFromSource(source, fallbackTitle, taxonomyKeys)
	bodyHash = hashing.GetBodyHash(source)
	return frontmatterHash, bodyHash
}

// DeterminePostChange compares current hashes with cache to determine change type.
func (m *Manager) DeterminePostChange(relPath, newFrontmatterHash, newBodyHash string) PostChangeType {
	if m.deps.Cache == nil {
		return PostChangeNew
	}
	meta, err := m.deps.Cache.GetItemByPath(relPath)
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

// rebuildNeighbors identifies and re-renders the immediate previous and next posts
// to ensure their navigation links (Title, etc.) stay in sync after a metadata change.
func (m *Manager) rebuildNeighbors(ctx context.Context, currentLink string) {
	metadataCtx, err := m.builder.GetContent().GetMetadataContext(ctx)
	if err != nil {
		return
	}

	items := metadataCtx.AllItems
	timeutil.SortItems(items)

	var currentItem models.ContentMetadata
	found := false
	for _, item := range items {
		if item.Link == currentLink {
			currentItem = item
			found = true
			break
		}
	}
	if !found {
		return
	}

	prev, next, _ := navigation.FindPrevNext(currentItem, items)

	// Re-render neighbors if they exist. We pass nil for source to force
	// the content service to read the latest source from disk.
	if prev != nil && prev.Path != "" {
		absPath := filepath.Join(m.cfg.ContentDir, prev.Path)
		if err := m.deps.Content.ProcessSingle(ctx, absPath, nil); err != nil {
			m.logger.Warn("Failed to re-render previous neighbor post", "path", prev.Path, "error", err)
		}
	}
	if next != nil && next.Path != "" {
		absPath := filepath.Join(m.cfg.ContentDir, next.Path)
		if err := m.deps.Content.ProcessSingle(ctx, absPath, nil); err != nil {
			m.logger.Warn("Failed to re-render next neighbor post", "path", next.Path, "error", err)
		}
	}
}

// HandleDeleteChange processes a deleted markdown file incrementally.
func (m *Manager) HandleDeleteChange(ctx context.Context, path string) {
	m.builder.LockBuild()
	defer m.builder.UnlockBuild()

	m.logger.Info("Content deletion detected, processing incremental removal...", "path", path)
	m.builder.RefreshBuildSession()

	relPath, htmlRelPath, _, err := m.ResolveContentPaths(path)
	if err != nil {
		m.logger.Warn("Failed to resolve paths for deleted file", "path", path, "error", err)
	}

	// 1. Identify neighbors BEFORE removing the item from the cache
	prevPath, nextPath := m.identifyNeighborsForDeletion(ctx, relPath)

	// 2. Remove generated artifacts for this Content
	m.removeDeletedOutputs(relPath, htmlRelPath)

	// 3. Remove from Cache and Search Index
	m.DeleteItemFromCache(path)
	if m.deps.Content != nil {
		m.deps.Content.WaitForCacheCommit()
	}

	// 4. Re-render neighbors AFTER the deleted item is gone from the cache
	m.rebuildDeletedNeighbors(ctx, prevPath, nextPath)

	// 5. Update Site-Wide Generators (RSS, Sitemap, Pagination)
	if err := m.renderIncrementalSiteWide(ctx); err != nil {
		m.logger.Error("Failed to update site-wide assets after deletion", "error", err)
	}

	// 6. Commit and save caches
	if err := m.commitIncrementalBuild(ctx, path); err != nil {
		m.logger.Error("Failed to commit after deletion", "error", err)
	}
	m.builder.SaveCaches()
	m.deps.Render.ClearRenderedFiles()

	// 7. Trigger WASM search index rebuild
	if watchCoordinator := m.builder.GetWatch(); watchCoordinator != nil {
		watchCoordinator.TriggerSearchRegeneration()
	}
}

func (m *Manager) resolveIncrementalParseInputs(path, relPath string, source []byte, newBodyHash string) (map[string]any, int, int) {
	preParsedMeta, bodyOffset, err := scanner.BuildCascadedMetadataForPath(m.sourceFs, m.cfg, path, source)
	if err != nil {
		m.logger.Debug("Failed to resolve cascaded metadata during incremental parse", "path", path, "error", err)
	}

	knownReadingTime := 0
	if m.deps.Cache != nil {
		if meta, cacheErr := m.deps.Cache.GetItemByPath(relPath); cacheErr == nil && meta != nil && meta.BodyHash == newBodyHash && meta.ReadingTime > 0 {
			knownReadingTime = meta.ReadingTime
		}
	}

	return preParsedMeta, bodyOffset, knownReadingTime
}

func (m *Manager) removeDeletedOutputs(relPath, htmlRelPath string) {
	if htmlRelPath == "" {
		return
	}

	outPath := filepath.Join(m.cfg.OutputDir, htmlRelPath)
	if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("Failed to remove deleted html file", "path", outPath, "error", err)
	}

	if m.cfg.Features.UseRawMarkdown {
		rawPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".md"
		if err := os.Remove(rawPath); err != nil && !os.IsNotExist(err) {
			m.logger.Warn("Failed to remove deleted raw markdown output", "path", rawPath, "error", err)
		}
	}

	hash := ""
	if m.deps.Cache != nil && relPath != "" {
		if cachedHash, err := m.deps.Cache.GetSocialCardHash(relPath); err == nil {
			hash = cachedHash
		}
	}

	cardHashes := []string{hash}
	if hash != "" {
		cardHashes = append(cardHashes, "")
	}
	for _, cardHash := range cardHashes {
		_, cardOutPath, _ := navigation.CardPaths(m.cfg.BaseURL, m.cfg.OutputDir, htmlRelPath, cardHash)
		if err := os.Remove(cardOutPath); err != nil && !os.IsNotExist(err) {
			m.logger.Warn("Failed to remove deleted social card output", "path", cardOutPath, "error", err)
		}
	}

	if hash != "" {
		cachedCard := filepath.Join(m.cfg.CacheDir, "social-cards", hash+".webp")
		if err := os.Remove(cachedCard); err != nil && !os.IsNotExist(err) {
			m.logger.Warn("Failed to remove deleted cached social card", "path", cachedCard, "error", err)
		}
	}
}

func (m *Manager) identifyNeighborsForDeletion(ctx context.Context, relPath string) (prevPath, nextPath string) {
	if m.deps.Cache == nil || relPath == "" {
		return
	}
	meta, err := m.deps.Cache.GetItemByPath(relPath)
	if err != nil || meta == nil {
		return
	}
	metadataCtx, _ := m.builder.GetContent().GetMetadataContext(ctx)
	if metadataCtx == nil {
		return
	}
	items := metadataCtx.AllItems
	timeutil.SortItems(items)
	var currentItem models.ContentMetadata
	for _, item := range items {
		if item.Link == meta.Link {
			currentItem = item
			break
		}
	}
	if currentItem.Link != "" {
		prev, next, _ := navigation.FindPrevNext(currentItem, items)
		if prev != nil {
			prevPath = prev.Path
		}
		if next != nil {
			nextPath = next.Path
		}
	}
	return
}

func (m *Manager) rebuildDeletedNeighbors(ctx context.Context, prevPath, nextPath string) {
	if prevPath != "" {
		absPath := filepath.Join(m.cfg.ContentDir, prevPath)
		if err := m.deps.Content.ProcessSingle(ctx, absPath, nil); err != nil {
			m.logger.Warn("Failed to re-render previous neighbor post", "path", prevPath, "error", err)
		}
	}
	if nextPath != "" {
		absPath := filepath.Join(m.cfg.ContentDir, nextPath)
		if err := m.deps.Content.ProcessSingle(ctx, absPath, nil); err != nil {
			m.logger.Warn("Failed to re-render next neighbor post", "path", nextPath, "error", err)
		}
	}
	if m.deps.Content != nil && (prevPath != "" || nextPath != "") {
		m.deps.Content.WaitForCacheCommit()
	}
}
