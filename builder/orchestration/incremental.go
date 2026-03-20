package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/services"

	"github.com/spf13/afero"
)

func indexedPostStableKey(ip models.IndexedPost) string {
	if ip.SourcePath != "" {
		return fspkg.NormalizePath(ip.SourcePath)
	}
	return fspkg.NormalizePath(ip.Record.Link)
}

func dedupeIndexedPosts(posts []models.IndexedPost) []models.IndexedPost {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]models.IndexedPost, 0, len(posts))
	for _, ip := range posts {
		key := indexedPostStableKey(ip)
		if idx, ok := seen[key]; ok {
			result[idx] = ip
			continue
		}
		seen[key] = len(result)
		result = append(result, ip)
	}
	return result
}

func (b *Engine) normalizeWatchPath(path string) string {
	if b.Watch != nil {
		return b.Watch.NormalizeWatchPath(path)
	}
	wd, _ := os.Getwd()
	return fspkg.NormalizeWatchPath(path, wd)
}

func (b *Engine) isContentPath(path string) bool {
	if b.Watch != nil {
		return b.Watch.IsContentPath(path)
	}
	absPath, _ := fspkg.AbsNormalizePath(path)
	contentDir, _ := fspkg.AbsNormalizePath(b.Cfg.ContentDir)
	return fspkg.IsPathInOrSame(absPath, contentDir)
}

func (b *Engine) isAssetPath(path string) bool {
	if b.Watch != nil {
		return b.Watch.IsAssetPath(path)
	}
	absPath, _ := fspkg.AbsNormalizePath(path)
	staticDir, _ := fspkg.AbsNormalizePath(b.Cfg.StaticDir)
	siteStaticDir, _ := fspkg.AbsNormalizePath("static")
	return fspkg.IsPathInOrSame(absPath, staticDir) || fspkg.IsPathInOrSame(absPath, siteStaticDir)
}

func (b *Engine) invalidateForTemplate(templatePath string) []string {
	if b.Watch != nil {
		return b.Watch.InvalidateForTemplate(templatePath)
	}
	tp := fspkg.NormalizePath(templatePath)
	templateDir := fspkg.NormalizePath(b.Cfg.TemplateDir)
	staticDir := fspkg.NormalizePath(b.Cfg.StaticDir)
	if strings.HasPrefix(tp, templateDir) {
		relTmpl, _ := fspkg.SafeRel(b.Cfg.TemplateDir, templatePath)
		relTmpl = fspkg.NormalizePath(relTmpl)
		if relTmpl == "layout.html" {
			return nil
		}
		if b.Deps.Cache != nil {
			ids, err := b.Deps.Cache.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				posts, err := b.Deps.Cache.GetPostsByIDs(ids)
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

// BuildChanged queues a file change for processing (for watch mode).
// Changes are debounced and processed in batches to avoid fsnotify buffer overflow.
func (b *Engine) BuildChanged(ctx context.Context, changedPath string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Change detected: " + filepath.Base(changedPath) + " " + op.String())

	if watch.IsSearchSourcePath(changedPath) {
		b.Deps.Wasm.SetSearchSourceDirty(true)
	}

	if b.Watch != nil {
		b.Watch.EnqueueChange(changedPath, op)
	}
}

// PostChangeType represents the type of change detected in a post
type PostChangeType int

const (
	// PostChangeNone indicates no changes detected
	PostChangeNone PostChangeType = iota
	// PostChangeNew indicates a new post that doesn't exist in cache
	PostChangeNew
	// PostChangeFrontmatter indicates frontmatter changed (requires full build)
	PostChangeFrontmatter
	// PostChangeBody indicates only body changed (single post rebuild sufficient)
	PostChangeBody
)

// resolveContentPaths resolves various path formats for incremental builds.
// Returns relative path, version, and HTML paths.
func (b *Engine) resolveContentPaths(path string) (relPath, version, htmlRelPath, cleanHtmlRelPath string, err error) {
	contentRoot, err := filepath.Abs(b.Cfg.ContentDir)
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

// computePostHashes calculates frontmatter and body hashes for change detection.
func (b *Engine) computePostHashes(source []byte) (frontmatterHash, bodyHash string) {
	frontmatterHash, _ = hashing.GetFrontmatterHashFromSource(source)
	bodyHash = hashing.GetBodyHash(source)
	return frontmatterHash, bodyHash
}

// determinePostChange compares current hashes with cache to determine change type.
func (b *Engine) determinePostChange(relPath, newFrontmatterHash, newBodyHash string) PostChangeType {
	if b.Deps.Cache == nil {
		return PostChangeNew
	}
	meta, err := b.Deps.Cache.GetPostByPath(relPath)
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

// triggerFullBuild executes a full site rebuild and saves caches.
func (b *Engine) triggerFullBuild(ctx context.Context) error {
	if err := b.build(ctx); err != nil {
		return err
	}
	b.SaveCaches()
	return nil
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Engine) buildSinglePost(ctx context.Context, path string) {
	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		b.Logger.Error("Error reading file", "path", path, "error", err)
		if buildErr := b.triggerFullBuild(ctx); buildErr != nil {
			b.Logger.Error("Full build failed", "error", buildErr)
		}
		return
	}

	relPath, version, htmlRelPath, cleanHtmlRelPath, err := b.resolveContentPaths(path)
	if err != nil {
		b.Logger.Error("Path resolution failed", "error", err)
		return
	}

	b.Logger.Debug("incremental content path resolved", "path", path, "relative", relPath, "version", version)

	newFrontmatterHash, newBodyHash := b.computePostHashes(source)
	changeType := b.determinePostChange(relPath, newFrontmatterHash, newBodyHash)

	switch changeType {
	case PostChangeNew, PostChangeFrontmatter:
		DevLogRebuild("New or frontmatter change detected, running full build...")
		if err := b.triggerFullBuild(ctx); err != nil {
			b.Logger.Error("Build failed", "error", err)
		}
		return

	case PostChangeBody:
		DevLogRebuild("Content change detected, rebuilding single post...")

		// Parse markdown exactly once
		parseRes, err := services.ParseMarkdown(
			services.ParseConfig{
				Source:           source,
				Path:             path,
				Version:          version,
				CleanHtmlRelPath: cleanHtmlRelPath,
				HtmlRelPath:      htmlRelPath,
			},
			services.ParseContext{
				MdPool:         b.MdPool,
				Cfg:            b.Cfg,
				NativeRenderer: b.NativeRenderer,
				DiagramAdapter: b.Deps.Diagrams,
				MathBatchSize:  services.DefaultMathBatchSize,
			},
		)

		if err != nil {
			b.Logger.Error("Error parsing markdown", "path", path, "error", err)
			return
		}

		if err := b.Deps.Post.ProcessSingleWithResult(ctx, path, source, parseRes); err != nil {
			b.Logger.Error("Failed to process single post", "error", err)
			if err := b.triggerFullBuild(ctx); err != nil {
				b.Logger.Error("Build failed", "error", err)
				return
			}
		} else {
			// Update the search index cache for faster regeneration
			if b.Search != nil {
				b.Search.UpdateIndexedPostCache(relPath, parseRes)
			}
		}
		b.SaveCaches()

		// Regenerate search index using the updated cache
		if err := b.regenerateSearchIndex(ctx); err != nil {
			b.Logger.Error("Failed to regenerate search index", "error", err)
			// Don't fail the build, just log the error
		}

	case PostChangeNone:
		DevLogSkip("No changes detected, skipping...")
	}
}

func (b *Engine) deletePostFromCache(path string) {
	relPath, err := fspkg.SafeRel(b.Cfg.ContentDir, path)
	if err != nil {
		b.Logger.Error("Failed to get relative path for deletion", "path", path, "error", err)
		return
	}

	if b.Deps.Cache == nil {
		return
	}

	postID := cache.GeneratePostID("", relPath)
	if err := b.Deps.Cache.DeletePost(postID); err != nil {
		b.Logger.Error("Failed to delete post from cache", "postID", postID, "error", err)
		return
	}

	// Also prune from in-memory search index
	if b.Search != nil {
		b.Search.PruneDeletedPost(relPath)
	}

	b.Logger.Info("Removed deleted post from cache", "path", relPath)
	DevLogChange(relPath, "delete")
}

// buildSingleFileChange processes a single file change (called from build queue processor).
// The change has already been classified by WatchCoordinator.
// regenerateSearchIndex triggers asynchronous search index regeneration via the WatchCoordinator.
// The coordinator handles debouncing and lock management to prevent deadlocks.
func (b *Engine) regenerateSearchIndex(ctx context.Context) error {
	if b.Watch != nil {
		b.Watch.TriggerSearchRegeneration()
	}
	return nil
}

func (b *Engine) buildSingleFileChange(ctx context.Context, path string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Processing queued change: " + filepath.Base(path) + " " + op.String())

	b.Cfg.BuildVersion = time.Now().UnixNano()
	b.Deps.Render.ReloadTemplates()
	b.Deps.Render.SetAssetsGate(nil)
	b.Deps.Post.SetAssetsGate(nil)

	if b.Watch != nil {
		evt := b.Watch.ClassifyChange(path, op)
		switch evt.Type {
		case watch.ChangeTypeContent:
			b.handleMarkdownChange(ctx, path)
			return
		case watch.ChangeTypeAsset:
			b.handleAssetChange(ctx, path)
			return
		case watch.ChangeTypeDelete:
			b.handleMarkdownChange(ctx, path)
			return
		}
	}

	b.handleOtherChange(ctx, path)
}

func (b *Engine) handleMarkdownChange(ctx context.Context, path string) {
	// Start fresh session/tracking state
	b.refreshBuildSession()

	b.buildSinglePost(ctx, path)
	if err := b.Tx.Commit(ctx); err != nil {
		b.Logger.Error("Sync/Commit failed", "error", err)
		b.deletePostFromCache(path)
		return
	}
	b.Deps.Render.ClearRenderedFiles()
}

func (b *Engine) handleAssetChange(ctx context.Context, path string) {
	DevLogRebuild("CSS/JS changed, running full rebuild...")
	if err := b.buildAssetOnly(ctx); err != nil {
		b.Logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
}

func (b *Engine) handleOtherChange(ctx context.Context, path string) {
	if err := b.build(ctx); err != nil {
		b.Logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
}
