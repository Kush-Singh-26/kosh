package run

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils"

	"github.com/spf13/afero"
)

func indexedPostStableKey(ip models.IndexedPost) string {
	if ip.SourcePath != "" {
		return utils.NormalizePath(ip.SourcePath)
	}
	return utils.NormalizePath(ip.Record.Link)
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
	path = utils.NormalizePath(path)
	return strings.HasPrefix(path, "cmd/search/") || strings.HasPrefix(path, "builder/search/") || strings.HasPrefix(path, "builder/models/")
}

func (b *Builder) normalizeWatchPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	return utils.NormalizeWatchPath(path, wd)
}

func normalizeAbsoluteWatchPath(path string) string {
	if abs, err := utils.AbsNormalizePath(path); err == nil {
		return abs
	}
	return utils.NormalizePath(path)
}

func (b *Builder) isContentPath(path string) bool {
	path = normalizeAbsoluteWatchPath(path)
	contentDir := normalizeAbsoluteWatchPath(b.cfg.ContentDir)
	return utils.IsPathInOrSame(path, contentDir)
}

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	tp := utils.NormalizePath(templatePath)
	templateDir := utils.NormalizePath(b.cfg.TemplateDir)
	staticDir := utils.NormalizePath(b.cfg.StaticDir)
	if strings.HasPrefix(tp, templateDir) {
		relTmpl, _ := utils.SafeRel(b.cfg.TemplateDir, templatePath)
		relTmpl = utils.NormalizePath(relTmpl)

		if relTmpl == "layout.html" {
			return nil // Layout changes affect everything
		}

		if b.deps.Cache != nil {
			ids, err := b.deps.Cache.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				posts, err := b.deps.Cache.GetPostsByIDs(ids)
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
func (b *Builder) BuildChanged(ctx context.Context, changedPath string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Change detected: " + filepath.Base(changedPath) + " " + op.String())

	// Mark search source as dirty if search files changed
	if isSearchSourcePath(changedPath) {
		b.state.searchSourceDirty.Store(true)
	}

	// Queue the build request (non-blocking)
	select {
	case b.state.buildQueue <- buildRequest{paths: []string{changedPath}, op: op}:
		// Queued successfully
	default:
		// Queue full - merge with existing by updating operation for same path
		// This is ok - the queued request will process the latest state
	}
}

// isAssetPath checks if a path is within the static assets directories
func (b *Builder) isAssetPath(path string) bool {
	path = normalizeAbsoluteWatchPath(path)
	staticDir := normalizeAbsoluteWatchPath(b.cfg.StaticDir)
	siteStaticDir := normalizeAbsoluteWatchPath("static")

	return utils.IsPathInOrSame(path, staticDir) || utils.IsPathInOrSame(path, siteStaticDir)
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Builder) buildSinglePost(ctx context.Context, path string) {
	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		b.logger.Error("Error reading file", "path", path, "error", err)
		if buildErr := b.build(ctx); buildErr != nil {
			b.logger.Error("Full build failed", "error", buildErr)
		}
		return
	}

	contentRoot, err := filepath.Abs(b.cfg.ContentDir)
	if err != nil {
		b.logger.Error("Failed to resolve content directory", "dir", b.cfg.ContentDir, "error", err)
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		b.logger.Error("Failed to resolve changed path", "path", path, "error", err)
		return
	}
	relPath, err := filepath.Rel(contentRoot, absPath)
	if err != nil {
		b.logger.Error("Failed to compute content-relative path", "path", path, "error", err)
		return
	}
	relPath = utils.NormalizePath(relPath)
	version, relPath := utils.GetVersionFromPath(utils.NormalizePath(filepath.Join("content", relPath)))
	b.logger.Debug("incremental content path resolved", "path", path, "relative", relPath, "version", version)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	cleanHtmlRelPath := htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
	}

	// Optimized: Get hashes first without parsing
	newFrontmatterHash, _ := utils.GetFrontmatterHashFromSource(source)
	newBodyHash := utils.GetBodyHash(source)

	var exists bool
	var cachedFrontmatterHash, cachedBodyHash string

	if b.deps.Cache != nil {
		meta, err := b.deps.Cache.GetPostByPath(relPath)
		if err == nil && meta != nil {
			exists = true
			cachedFrontmatterHash = meta.ContentHash
			cachedBodyHash = meta.BodyHash
		}
	}

	// Check if frontmatter changed (requires full rebuild)
	frontmatterChanged := exists && cachedFrontmatterHash != newFrontmatterHash
	// Check if only body changed (single post rebuild sufficient)
	bodyOnlyChanged := exists && cachedFrontmatterHash == newFrontmatterHash && cachedBodyHash != newBodyHash

	if !exists {
		DevLogRebuild("New post detected, running full build...")
		if err := b.build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
		return
	}

	if frontmatterChanged {
		DevLogRebuild("Frontmatter changed, running full build...")
		if err := b.build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
		return
	}

	if bodyOnlyChanged || cachedBodyHash == "" {
		DevLogRebuild("Content change detected, rebuilding single post...")

		// Now we parse exactly once
		parseRes, err := services.ParseMarkdown(
			ctx,
			source,
			path,
			version,
			cleanHtmlRelPath,
			htmlRelPath,
			b.mdPool,
			b.cfg,
			b.nativeRenderer,
			b.deps.Diagrams,
			&b.mu,
			newFrontmatterHash,
			0,
			0,
			nil,
		)

		if err != nil {
			b.logger.Error("Error parsing markdown", "path", path, "error", err)
			return
		}

		if err := b.deps.Post.ProcessSingleWithResult(ctx, path, source, parseRes); err != nil {
			b.logger.Error("Failed to process single post", "error", err)
			if err := b.build(ctx); err != nil {
				b.logger.Error("Build failed", "error", err)
				return
			}
		} else {
			// Update the in-memory indexedPosts cache for faster search regeneration
			b.updateIndexedPostCache(relPath, parseRes)
		}
		b.SaveCaches()

		// Regenerate search index using the updated cache
		if err := b.regenerateSearchIndex(ctx); err != nil {
			b.logger.Error("Failed to regenerate search index", "error", err)
			// Don't fail the build, just log the error
		}
	} else {
		DevLogSkip("No changes detected, skipping...")
	}
}

func (b *Builder) deletePostFromCache(path string) {
	relPath, err := utils.SafeRel(b.cfg.ContentDir, path)
	if err != nil {
		b.logger.Error("Failed to get relative path for deletion", "path", path, "error", err)
		return
	}

	if b.deps.Cache == nil {
		return
	}

	postID := cache.GeneratePostID("", relPath)
	if err := b.deps.Cache.DeletePost(postID); err != nil {
		b.logger.Error("Failed to delete post from cache", "postID", postID, "error", err)
		return
	}

	// Also prune from in-memory search index
	b.mu.Lock()
	targetKey := utils.NormalizePath(relPath)
	newIndexed := make([]models.IndexedPost, 0, len(b.state.indexedPosts))
	for _, ip := range b.state.indexedPosts {
		if indexedPostStableKey(ip) != targetKey {
			newIndexed = append(newIndexed, ip)
		}
	}
	b.state.indexedPosts = newIndexed
	b.mu.Unlock()

	b.logger.Info("Removed deleted post from cache", "path", relPath)
	DevLogChange(relPath, "delete")
}

// updateIndexedPostCache updates a single entry in the in-memory cache
func (b *Builder) updateIndexedPostCache(relPath string, parseRes *services.ParsedMarkdownResult) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.state.indexedPosts) == 0 {
		return
	}

	found := false
	targetKey := utils.NormalizePath(relPath)
	for i, ip := range b.state.indexedPosts {
		if indexedPostStableKey(ip) == targetKey {
			// Update existing record
			b.state.indexedPosts[i] = models.IndexedPost{
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
		b.state.indexedPosts = append(b.state.indexedPosts, models.IndexedPost{
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
func (b *Builder) regenerateSearchIndex(ctx context.Context) error {
	// Non-blocking send to search index queue
	// If queue is full, request is already pending, so skip
	select {
	case b.state.searchIndexCh <- struct{}{}:
	default:
	}
	return nil
}

func (b *Builder) doRegenerateSearchIndex(ctx context.Context) error {
	if b.deps.Cache == nil {
		return nil
	}

	var indexedPosts []models.IndexedPost
	if len(b.state.indexedPosts) > 0 {
		// Use in-memory cache if available (very fast)
		indexedPosts = b.state.indexedPosts
	} else {
		// Fallback to BoltDB only if cache is empty
		postIDs, err := b.deps.Cache.ListAllPosts()
		if err != nil {
			return err
		}
		if len(postIDs) == 0 {
			return nil
		}
		posts, err := b.deps.Cache.GetPostsByIDs(postIDs)
		if err != nil {
			return err
		}
		searchRecords, err := b.deps.Cache.GetSearchRecords(postIDs)
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
		b.state.indexedPosts = indexedPosts
	}

	if len(indexedPosts) == 0 {
		return nil
	}
	indexedPosts = dedupeIndexedPosts(indexedPosts)
	b.state.indexedPosts = indexedPosts

	// Generate search index file
	path, err := generators.GenerateSearchIndex(b.Sink, b.cfg.OutputDir, indexedPosts)
	if err != nil {
		return err
	}
	b.deps.Render.RegisterFile(path)

	return nil
}

// processSearchIndexQueue processes search index regeneration requests from the channel.
// It debounces multiple requests and acquires buildMu safely to avoid deadlocks.
// Search regeneration is fire-and-forget: errors are logged but don't block the build,
// as the search index can be rebuilt on next full build.
func (b *Builder) processSearchIndexQueue() {
	var pending bool
	var timer *time.Timer
	var timerRunning bool

	for range b.state.searchIndexCh {
		// Mark as pending
		pending = true

		// Start or reset debounce timer
		if !timerRunning {
			// Calculate delay: 500ms since last run, or 100ms for burst start
			delay := 500 * time.Millisecond
			if time.Since(b.state.lastSearchIndexRegeneration) > 2*time.Second {
				delay = 100 * time.Millisecond
			}

			timer = time.AfterFunc(delay, func() {
				// Timer fired, try to acquire lock
				if pending {
					pending = false
					// Use goroutine to avoid blocking if lock is held
					go func() {
						b.state.buildMu.Lock()
						defer b.state.buildMu.Unlock()

						// Perform the actual regeneration
						b.state.lastSearchIndexRegeneration = time.Now()
						if err := b.doRegenerateSearchIndex(context.Background()); err != nil {
							b.logger.Error("Search index regeneration failed", "error", err)
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

// processBuildQueue processes build requests from the watch mode queue.
// It debounces multiple file changes and processes them in batches.
func (b *Builder) processBuildQueue() {
	var mergedPaths map[string]fsnotify.Op
	debounce := time.NewTimer(100 * time.Millisecond)
	defer debounce.Stop()

	for {
		select {
		case req, ok := <-b.state.buildQueue:
			if !ok {
				// Channel closed, process any pending and exit
				if len(mergedPaths) > 0 {
					b.state.buildMu.Lock()
					for path, op := range mergedPaths {
						b.buildSingleFileChange(context.Background(), path, op)
					}
					b.state.buildMu.Unlock()
				}
				return
			}
			// Merge requests for the same path
			if mergedPaths == nil {
				mergedPaths = make(map[string]fsnotify.Op)
			}
			for _, path := range req.paths {
				// Keep the most recent operation for each path
				mergedPaths[path] = req.op
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
				b.state.buildMu.Lock()
				for path, op := range mergedPaths {
					b.buildSingleFileChange(context.Background(), path, op)
				}
				b.state.buildMu.Unlock()
				// Reset for next batch
				mergedPaths = nil
			}
		}
	}
}

// buildSingleFileChange processes a single file change (called from build queue processor)
func (b *Builder) buildSingleFileChange(ctx context.Context, path string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Processing queued change: " + filepath.Base(path) + " " + op.String())

	// Reset rendering gates on any change
	b.deps.Render.SetAssetsGate(nil)
	b.deps.Post.SetAssetsGate(nil)

	// Handle markdown files - single post rebuild
	if strings.HasSuffix(strings.ToLower(path), ".md") && b.isContentPath(path) {
		b.cfg.BuildVersion = time.Now().UnixNano()
		b.deps.Render.ReloadTemplates()
		b.deps.Post.SetAssetsGate(nil)

		// Start fresh session/tracking state
		b.refreshBuildSession()

		b.buildSinglePost(ctx, path)
		if err := b.Tx.Commit(ctx); err != nil {
			b.logger.Error("Sync/Commit failed", "error", err)
			b.deletePostFromCache(path)
			return
		}
		b.deps.Render.ClearRenderedFiles()
		return
	}

	// Handle CSS/JS changes - do full rebuild to update HTML with new asset hashes
	ext := strings.ToLower(filepath.Ext(path))
	if (ext == ".css" || ext == ".js") && b.isAssetPath(path) {
		DevLogRebuild("CSS/JS changed, running full rebuild...")
		b.cfg.BuildVersion = time.Now().UnixNano()
		b.deps.Render.ReloadTemplates()
		b.deps.Post.SetAssetsGate(nil)
		if err := b.buildAssetOnly(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
		return
	}

	// Everything else - full rebuild
	b.cfg.BuildVersion = time.Now().UnixNano()
	b.deps.Render.ReloadTemplates()
	b.deps.Post.SetAssetsGate(nil)
	if err := b.build(ctx); err != nil {
		b.logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
}
