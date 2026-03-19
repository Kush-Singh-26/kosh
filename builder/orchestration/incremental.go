package orchestration

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/fsnotify/fsnotify"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
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

func isSearchSourcePath(path string) bool {
	path = fspkg.NormalizePath(path)
	return strings.HasPrefix(path, "cmd/search/") || strings.HasPrefix(path, "builder/search/") || strings.HasPrefix(path, "builder/models/")
}

func (b *Engine) normalizeWatchPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	return fspkg.NormalizeWatchPath(path, wd)
}

func normalizeAbsoluteWatchPath(path string) string {
	if abs, err := fspkg.AbsNormalizePath(path); err == nil {
		return abs
	}
	return fspkg.NormalizePath(path)
}

func (b *Engine) isContentPath(path string) bool {
	path = normalizeAbsoluteWatchPath(path)
	contentDir := normalizeAbsoluteWatchPath(b.Cfg.ContentDir)
	return fspkg.IsPathInOrSame(path, contentDir)
}

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Engine) invalidateForTemplate(templatePath string) []string {
	tp := fspkg.NormalizePath(templatePath)
	templateDir := fspkg.NormalizePath(b.Cfg.TemplateDir)
	staticDir := fspkg.NormalizePath(b.Cfg.StaticDir)
	if strings.HasPrefix(tp, templateDir) {
		relTmpl, _ := fspkg.SafeRel(b.Cfg.TemplateDir, templatePath)
		relTmpl = fspkg.NormalizePath(relTmpl)

		if relTmpl == "layout.html" {
			return nil // Layout changes affect everything
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

	// Mark search source as dirty if search files changed
	if isSearchSourcePath(changedPath) {
		b.Deps.Wasm.SetSearchSourceDirty(true)
	}

	// Queue the build request (non-blocking)
	select {
	case b.State.BuildQueue <- BuildRequest{Paths: []string{changedPath}, Op: op}:
		// Queued successfully
	default:
		// Queue full - merge with existing by updating operation for same path
		// This is ok - the queued request will process the latest state
	}
}

// isAssetPath checks if a path is within the static assets directories
func (b *Engine) isAssetPath(path string) bool {
	path = normalizeAbsoluteWatchPath(path)
	staticDir := normalizeAbsoluteWatchPath(b.Cfg.StaticDir)
	siteStaticDir := normalizeAbsoluteWatchPath("static")

	return fspkg.IsPathInOrSame(path, staticDir) || fspkg.IsPathInOrSame(path, siteStaticDir)
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
			// Update the in-memory indexedPosts cache for faster search regeneration
			b.updateIndexedPostCache(relPath, parseRes)
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
	targetKey := fspkg.NormalizePath(relPath)
	newIndexed := make([]models.IndexedPost, 0, len(b.State.IndexedPosts))
	for _, ip := range b.State.IndexedPosts {
		if indexedPostStableKey(ip) != targetKey {
			newIndexed = append(newIndexed, ip)
		}
	}
	b.State.IndexedPosts = newIndexed

	b.Logger.Info("Removed deleted post from cache", "path", relPath)
	DevLogChange(relPath, "delete")
}

// updateIndexedPostCache updates a single entry in the in-memory cache
func (b *Engine) updateIndexedPostCache(relPath string, parseRes *services.ParsedMarkdownResult) {
	if len(b.State.IndexedPosts) == 0 {
		return
	}

	found := false
	targetKey := fspkg.NormalizePath(relPath)
	for i, ip := range b.State.IndexedPosts {
		if indexedPostStableKey(ip) == targetKey {
			// Update existing record
			b.State.IndexedPosts[i] = models.IndexedPost{
				Record:          parseRes.SearchRecord,
				SourcePath:      targetKey,
				WordFreqs:       parseRes.WordFreqs,
				DocLen:          parseRes.DocLen,
				StemMap:         parseRes.StemMap,
				PositionalIndex: parseRes.PositionalIndex,
			}
			found = true
			break
		}
	}

	if !found {
		// New post added to existing cache
		b.State.IndexedPosts = append(b.State.IndexedPosts, models.IndexedPost{
			Record:          parseRes.SearchRecord,
			SourcePath:      targetKey,
			WordFreqs:       parseRes.WordFreqs,
			DocLen:          parseRes.DocLen,
			StemMap:         parseRes.StemMap,
			PositionalIndex: parseRes.PositionalIndex,
		})
	}
}

// regenerateSearchIndex rebuilds the search index from cached post data.
// It uses a channel-based debouncer to avoid deadlocks when buildMu is held.
// regenerateSearchIndex triggers asynchronous search index regeneration.
// Uses a buffered channel (capacity 1) to debounce rapid updates.
// The channel is non-blocking: if a regeneration is already pending,
// new requests are dropped as the pending one will process the latest state.
// This prevents deadlocks when BuildMu is held during incremental builds.
func (b *Engine) regenerateSearchIndex(ctx context.Context) error {
	// Non-blocking send to search index queue
	// If queue is full, request is already pending, so skip
	select {
	case b.State.SearchIndexCh <- struct{}{}:
	default:
	}
	return nil
}

func (b *Engine) doRegenerateSearchIndex(ctx context.Context) error {
	if b.Deps.Cache == nil {
		return nil
	}

	var indexedPosts []models.IndexedPost
	if len(b.State.IndexedPosts) > 0 {
		// Use in-memory cache if available (very fast)
		indexedPosts = b.State.IndexedPosts
	} else {
		// Fallback to BoltDB only if cache is empty
		postIDs, err := b.Deps.Cache.ListAllPosts()
		if err != nil {
			return err
		}
		if len(postIDs) == 0 {
			return nil
		}
		posts, err := b.Deps.Cache.GetPostsByIDs(postIDs)
		if err != nil {
			return err
		}
		searchRecords, err := b.Deps.Cache.GetSearchRecords(postIDs)
		if err != nil {
			return err
		}

		sort.Strings(postIDs)
		indexedPosts = make([]models.IndexedPost, 0, len(posts))
		for _, postID := range postIDs {
			postMeta, ok := posts[postID]
			if !ok || postMeta == nil {
				continue
			}
			searchRec, ok := searchRecords[postID]
			if !ok || searchRec == nil {
				continue
			}
			htmlRelPath := strings.ToLower(strings.Replace(postMeta.Path, ".md", ".html", 1))
			indexedPosts = append(indexedPosts, models.IndexedPost{
				Record: models.PostRecord{
					ID:              xxh3.HashString(htmlRelPath),
					Title:           postMeta.Title,
					NormalizedTitle: searchRec.NormalizedTitle,
					Link:            htmlRelPath,
					Description:     postMeta.Description,
					Tags:            postMeta.Tags,
					NormalizedTags:  searchRec.NormalizedTags,
					Version:         postMeta.Version,
				},
				SourcePath:      postMeta.Path,
				WordFreqs:       searchRec.BM25Data,
				DocLen:          searchRec.DocLen,
				StemMap:         searchRec.StemMap,
				PositionalIndex: searchRec.PositionalIndex,
			})
		}
		// Warm the cache
		b.State.IndexedPosts = indexedPosts
	}

	if len(indexedPosts) == 0 {
		return nil
	}
	indexedPosts = dedupeIndexedPosts(indexedPosts)
	b.State.IndexedPosts = indexedPosts

	// Generate search index file
	path, err := generators.GenerateSearchIndex(b.Sink, b.Cfg.OutputDir, indexedPosts)
	if err != nil {
		return err
	}
	b.Deps.Render.RegisterFile(path)

	return nil
}

// ProcessSearchIndexQueue processes search index regeneration requests from the channel.
// It debounces multiple requests and acquires buildMu safely to avoid deadlocks.
// Search regeneration is fire-and-forget: errors are logged but don't block the build,
// as the search index can be rebuilt on next full build.
// processSearchIndexQueue implements timer-based debouncing for search regeneration.
// Uses a 500ms debounce (100ms for burst start) to coalesce rapid changes.
// The goroutine acquires BuildMu to safely access IndexedPosts and regenerate.
// This design prevents:
// 1. Deadlocks when BuildMu is already held by incremental builds
// 2. Excessive regeneration during rapid file changes
// 3. Stale search indices after multiple updates
func (b *Engine) processSearchIndexQueue() {
	var pending bool
	var timer *time.Timer
	var timerRunning bool

	for range b.State.SearchIndexCh {
		// Mark as pending
		pending = true

		// Start or reset debounce timer
		if !timerRunning {
			// Calculate delay: 500ms since last run, or 100ms for burst start
			delay := 500 * time.Millisecond
			if time.Since(b.State.LastSearchIndexRegeneration) > 2*time.Second {
				delay = 100 * time.Millisecond
			}

			timer = time.AfterFunc(delay, func() {
				// Timer fired, try to acquire lock
				if pending {
					pending = false
					// Use goroutine to avoid blocking if lock is held
					go func() {
						b.State.BuildMu.Lock()
						defer b.State.BuildMu.Unlock()

						// Perform the actual regeneration
						b.State.LastSearchIndexRegeneration = time.Now()
						if err := b.doRegenerateSearchIndex(context.Background()); err != nil {
							b.Logger.Error("Search index regeneration failed", "error", err)
						}
					}()
				}
			})
			timerRunning = true
		}
	}

	// Channel closed, cleanup
	if timer != nil && timerRunning {
		timer.Stop()
	}
}

// ProcessBuildQueue processes build requests from the watch mode queue.
// It debounces multiple file changes and processes them in batches.
// processBuildQueue implements file change batching for watch mode.
// Merges multiple file changes within a 100ms debounce window.
// Uses a map to track the latest operation per path.
// This prevents:
// 1. fsnotify buffer overflow from rapid changes
// 2. Redundant rebuilds for the same file
// 3. Race conditions between concurrent file modifications
func (b *Engine) processBuildQueue() {
	var mergedPaths map[string]fsnotify.Op
	debounce := time.NewTimer(100 * time.Millisecond)
	defer debounce.Stop()

	for {
		select {
		case req, ok := <-b.State.BuildQueue:
			if !ok {
				// Channel closed, process any pending and exit
				if len(mergedPaths) > 0 {
					b.State.BuildMu.Lock()
					for path, op := range mergedPaths {
						b.buildSingleFileChange(context.Background(), path, op)
					}
					b.State.BuildMu.Unlock()
				}
				return
			}
			// Merge requests for the same path
			if mergedPaths == nil {
				mergedPaths = make(map[string]fsnotify.Op)
			}
			for _, path := range req.Paths {
				// Keep the most recent operation for each path
				mergedPaths[path] = req.Op
			}
			// Reset debounce timer
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(100 * time.Millisecond)

		case <-debounce.C:
			// Process pending builds
			if len(mergedPaths) > 0 {
				b.State.BuildMu.Lock()
				for path, op := range mergedPaths {
					b.buildSingleFileChange(context.Background(), path, op)
				}
				b.State.BuildMu.Unlock()
				// Reset for next batch
				mergedPaths = nil
			}
		}
	}
}

// buildSingleFileChange processes a single file change (called from build queue processor)
func (b *Engine) buildSingleFileChange(ctx context.Context, path string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Processing queued change: " + filepath.Base(path) + " " + op.String())

	// Common setup for any change
	b.Cfg.BuildVersion = time.Now().UnixNano()
	b.Deps.Render.ReloadTemplates()
	b.Deps.Render.SetAssetsGate(nil)
	b.Deps.Post.SetAssetsGate(nil)

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" && b.isContentPath(path) {
		b.handleMarkdownChange(ctx, path)
		return
	}

	if (ext == ".css" || ext == ".js") && b.isAssetPath(path) {
		b.handleAssetChange(ctx, path)
		return
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
