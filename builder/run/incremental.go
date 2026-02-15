package run

import (
	"context"
	"path/filepath"
	"strings"

	gParser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/utils"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark-meta"
)

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
func (b *Builder) BuildChanged(ctx context.Context, changedPath string) {
	// Prevent concurrent builds - critical for stability during rapid changes
	b.buildMu.Lock()
	defer b.buildMu.Unlock()

	select {
	case <-ctx.Done():
		return
	default:
	}

	b.logger.Info("⚡ Change detected", "path", changedPath)

	// Handle markdown files - single post rebuild
	if strings.HasSuffix(changedPath, ".md") && strings.HasPrefix(changedPath, b.cfg.ContentDir) {
		b.buildSinglePost(ctx, changedPath)
		if err := utils.SyncVFS(b.DestFs, b.cfg.OutputDir, b.renderService.GetRenderedFiles()); err != nil {
			b.logger.Error("Sync failed", "error", err)
			return
		}
		b.renderService.ClearRenderedFiles()
		return
	}

	// Handle CSS/JS changes - do full rebuild to update HTML with new asset hashes
	ext := strings.ToLower(filepath.Ext(changedPath))
	if (ext == ".css" || ext == ".js") && b.isAssetPath(changedPath) {
		b.logger.Info("🎨 CSS/JS changed, running full rebuild...")
		if err := b.Build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
		return
	}

	// Everything else - full rebuild
	if err := b.Build(ctx); err != nil {
		b.logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
}

// isAssetPath checks if a path is within the static assets directories
func (b *Builder) isAssetPath(path string) bool {
	path = filepath.ToSlash(path)
	staticDir := filepath.ToSlash(b.cfg.StaticDir)
	siteStaticDir := "static"

	return strings.HasPrefix(path, staticDir) || strings.HasPrefix(path, siteStaticDir)
}

// buildSinglePost rebuilds only the changed post with smart change detection
func (b *Builder) buildSinglePost(ctx context.Context, path string) {
	source, err := afero.ReadFile(b.SourceFs, path)
	if err != nil {
		b.logger.Error("Error reading file", "path", path, "error", err)
		if buildErr := b.Build(ctx); buildErr != nil {
			b.logger.Error("Full build failed", "error", buildErr)
		}
		return
	}

	context := gParser.NewContext()
	context.Set(mdParser.ContextKeyFilePath, path)
	reader := text.NewReader(source)
	b.md.Parser().Parse(reader, gParser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)
	newBodyHash := utils.GetBodyHash(source)

	relPath, _ := utils.SafeRel(b.cfg.ContentDir, path)

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
		if err := b.Build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
	} else if frontmatterChanged {
		b.logger.Info("🏷️  Frontmatter changed, running full build...")
		if err := b.Build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
	} else if bodyOnlyChanged || cachedBodyHash == "" {
		b.logger.Info("📝 Content-only change detected, rebuilding single post...")
		if err := b.postService.ProcessSingle(ctx, path); err != nil {
			b.logger.Error("Failed to process single post", "error", err)
			if err := b.Build(ctx); err != nil {
				b.logger.Error("Build failed", "error", err)
				return
			}
		}
		b.SaveCaches()
	} else {
		b.logger.Info("✅ No changes detected, skipping...")
	}
}
