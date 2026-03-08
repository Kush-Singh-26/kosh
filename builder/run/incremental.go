package run

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils"

	"github.com/spf13/afero"
)

func (b *Builder) normalizeWatchPath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				path = rel
			}
		}
	}
	return filepath.ToSlash(path)
}

func (b *Builder) isContentPath(path string) bool {
	path = b.normalizeWatchPath(path)
	contentDir := filepath.ToSlash(filepath.Clean(b.cfg.ContentDir))
	return strings.HasPrefix(path, contentDir+"/") || path == contentDir
}

// invalidateForTemplate determines which posts to invalidate based on changed template
func (b *Builder) invalidateForTemplate(templatePath string) []string {
	tp := filepath.ToSlash(templatePath)
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.TemplateDir)) {
		relTmpl, _ := utils.SafeRel(b.cfg.TemplateDir, tp)
		relTmpl = filepath.ToSlash(relTmpl)

		if relTmpl == "layout.html" {
			return nil // Layout changes affect everything
		}

		if b.cacheService != nil {
			ids, err := b.cacheService.GetPostsByTemplate(relTmpl)
			if err == nil && len(ids) > 0 {
				posts, err := b.cacheService.GetPostsByIDs(ids)
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
	if strings.HasPrefix(tp, filepath.ToSlash(b.cfg.StaticDir)) {
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

// BuildChanged rebuilds only the changed file (for watch mode)
func (b *Builder) BuildChanged(ctx context.Context, changedPath string, op fsnotify.Op) {
	b.buildMu.Lock()
	defer b.buildMu.Unlock()

	select {
	case <-ctx.Done():
		return
	default:
	}

	b.logger.Info("⚡ Change detected", "path", changedPath, "op", op.String())

	// Handle file deletion - remove from cache
	if op&fsnotify.Remove == fsnotify.Remove || op&fsnotify.Rename == fsnotify.Rename {
		if strings.HasSuffix(strings.ToLower(changedPath), ".md") && b.isContentPath(changedPath) {
			b.deletePostFromCache(changedPath)
			if err := b.build(ctx); err != nil {
				b.logger.Error("Build failed after deletion", "error", err)
				return
			}
			b.SaveCaches()
			// Search index is regenerated during Build()
			return
		}
	}

	// Handle markdown files - single post rebuild
	if strings.HasSuffix(strings.ToLower(changedPath), ".md") && b.isContentPath(changedPath) {
		b.cfg.BuildVersion = time.Now().UnixNano()
		b.renderService.ReloadTemplates()
		b.postService.SetAssetsGate(nil)
		b.Tx = utils.NewBuildTransaction(b.cfg.OutputDir, false)
		b.Sink = utils.NewDiskSink(b.Tx.StagingDir(), b.cfg.OutputDir)
		b.renderService.SetSink(b.Sink)
		b.assetService.SetSink(b.Sink)
		b.postService.SetSink(b.Sink)

		b.buildSinglePost(ctx, changedPath)
		if err := b.Tx.Commit(); err != nil {
			b.logger.Error("Sync/Commit failed", "error", err)
			b.deletePostFromCache(changedPath)
			return
		}
		b.renderService.ClearRenderedFiles()
		return
	}

	// Handle CSS/JS changes - do full rebuild to update HTML with new asset hashes
	ext := strings.ToLower(filepath.Ext(changedPath))
	if (ext == ".css" || ext == ".js") && b.isAssetPath(changedPath) {
		b.logger.Info("🎨 CSS/JS changed, running full rebuild...")
		b.cfg.BuildVersion = time.Now().UnixNano()
		b.renderService.ReloadTemplates()
		b.postService.SetAssetsGate(nil)
		if err := b.build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
		return
	}

	// Everything else - full rebuild
	b.cfg.BuildVersion = time.Now().UnixNano()
	b.renderService.ReloadTemplates()
	b.postService.SetAssetsGate(nil)
	if err := b.build(ctx); err != nil {
		b.logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
}

// isAssetPath checks if a path is within the static assets directories
func (b *Builder) isAssetPath(path string) bool {
	path = b.normalizeWatchPath(path)
	staticDir := filepath.ToSlash(filepath.Clean(b.cfg.StaticDir))
	siteStaticDir := "static"

	return strings.HasPrefix(path, staticDir+"/") || path == staticDir || strings.HasPrefix(path, siteStaticDir+"/") || path == siteStaticDir
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
	relPath = filepath.ToSlash(relPath)
	version, relPath := utils.GetVersionFromPath(filepath.ToSlash(filepath.Join("content", relPath)))
	b.logger.Debug("incremental content path resolved", "path", path, "relative", relPath, "version", version)
	htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))
	cleanHtmlRelPath := htmlRelPath
	if version != "" {
		cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, strings.ToLower(version)+"/")
	}

	// First parse to check hashes
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
		b.diagramAdapter,
		&b.mu,
	)

	if err != nil {
		b.logger.Error("Error parsing markdown", "path", path, "error", err)
		return
	}

	newFrontmatterHash := parseRes.FrontmatterHash
	newBodyHash := utils.GetBodyHash(source)

	var exists bool
	var cachedFrontmatterHash, cachedBodyHash string

	if b.cacheService != nil {
		meta, err := b.cacheService.GetPostByPath(relPath)
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
		b.logger.Info("🆕 New post detected, running full build...")
		if err := b.build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
	} else if frontmatterChanged {
		b.logger.Info("🏷️  Frontmatter changed, running full build...")
		if err := b.build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
	} else if bodyOnlyChanged || cachedBodyHash == "" {
		b.logger.Info("📝 Content-only change detected, rebuilding single post (Zero-Double-Parse)...")
		if err := b.postService.ProcessSingleWithResult(ctx, path, source, parseRes); err != nil {
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
		b.logger.Info("✅ No changes detected, skipping...", "path", relPath)
	}
}

func (b *Builder) deletePostFromCache(path string) {
	relPath, err := utils.SafeRel(b.cfg.ContentDir, path)
	if err != nil {
		b.logger.Error("Failed to get relative path for deletion", "path", path, "error", err)
		return
	}

	if b.cacheService == nil {
		return
	}

	postID := cache.GeneratePostID("", relPath)
	if err := b.cacheService.DeletePost(postID); err != nil {
		b.logger.Error("Failed to delete post from cache", "postID", postID, "error", err)
		return
	}

	b.logger.Info("🗑️ Removed deleted post from cache", "path", relPath)
}

// lastSearchIndexRegeneration tracks when we last regenerated the search index
var lastSearchIndexRegeneration atomic.Value // stores time.Time

// updateIndexedPostCache updates a single entry in the in-memory cache
func (b *Builder) updateIndexedPostCache(relPath string, parseRes *services.ParsedMarkdownResult) {
	if len(b.indexedPosts) == 0 {
		return
	}

	found := false
	for i, ip := range b.indexedPosts {
		if ip.Record.Link == parseRes.SearchRecord.Link {
			// Update existing record
			b.indexedPosts[i] = models.IndexedPost{
				Record:          parseRes.SearchRecord,
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
		b.indexedPosts = append(b.indexedPosts, models.IndexedPost{
			Record:          parseRes.SearchRecord,
			WordFreqs:       parseRes.WordFreqs,
			DocLen:          parseRes.DocLen,
			StemMap:         parseRes.StemMap,
			PositionalIndex: parseRes.PositionalIndex,
		})
	}
}

// regenerateSearchIndex rebuilds the search index from cached post data.
// It uses debouncing and the in-memory cache to avoid redundant BoltDB operations.
func (b *Builder) regenerateSearchIndex(ctx context.Context) error {
	// Debounce: don't regenerate if less than 500ms since last one
	if last, ok := lastSearchIndexRegeneration.Load().(time.Time); ok && time.Since(last) < 500*time.Millisecond {
		return nil
	}
	lastSearchIndexRegeneration.Store(time.Now())

	if b.cacheService == nil {
		return nil
	}

	var indexedPosts []models.IndexedPost
	if len(b.indexedPosts) > 0 {
		// Use in-memory cache if available (very fast)
		indexedPosts = b.indexedPosts
	} else {
		// Fallback to BoltDB only if cache is empty
		postIDs, err := b.cacheService.ListAllPosts()
		if err != nil {
			return err
		}
		if len(postIDs) == 0 {
			return nil
		}
		posts, err := b.cacheService.GetPostsByIDs(postIDs)
		if err != nil {
			return err
		}
		searchRecords, err := b.cacheService.GetSearchRecords(postIDs)
		if err != nil {
			return err
		}

		sort.Strings(postIDs)
		indexedPosts = make([]models.IndexedPost, 0, len(posts))
		id := 0
		for _, postID := range postIDs {
			postMeta, ok := posts[postID]
			if !ok || postMeta == nil {
				continue
			}
			searchRec, ok := searchRecords[postID]
			if !ok || searchRec == nil {
				continue
			}
			id++
			indexedPosts = append(indexedPosts, models.IndexedPost{
				Record: models.PostRecord{
					ID:              id,
					Title:           postMeta.Title,
					NormalizedTitle: searchRec.NormalizedTitle,
					Link:            postMeta.Link,
					Description:     postMeta.Description,
					Tags:            postMeta.Tags,
					NormalizedTags:  searchRec.NormalizedTags,
					Version:         postMeta.Version,
				},
				WordFreqs:       searchRec.BM25Data,
				DocLen:          searchRec.DocLen,
				StemMap:         searchRec.StemMap,
				PositionalIndex: searchRec.PositionalIndex,
			})
		}
		// Warm the cache
		b.indexedPosts = indexedPosts
	}

	if len(indexedPosts) == 0 {
		return nil
	}

	// Generate search index file
	start := time.Now()
	path, err := generators.GenerateSearchIndex(b.Sink, b.cfg.OutputDir, indexedPosts)
	if err != nil {
		return err
	}
	b.renderService.RegisterFile(path)

	b.logger.Info("🔍 Search index regenerated",
		"posts", len(indexedPosts),
		"duration", time.Since(start).Round(time.Millisecond))
	return nil
}
