package run

import (
	"context"
	"path/filepath"
	"strings"

	gParser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/utils"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"
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
	select {
	case <-ctx.Done():
		return
	default:
	}

	if strings.HasSuffix(changedPath, ".md") && strings.HasPrefix(changedPath, b.cfg.ContentDir) {
		b.buildSinglePost(ctx, changedPath)
		if err := utils.SyncVFS(b.DestFs, b.cfg.OutputDir, b.renderService.GetRenderedFiles()); err != nil {
			b.logger.Error("Sync failed", "error", err)
			return
		}
		b.renderService.ClearRenderedFiles()
		return
	}

	if err := b.Build(ctx); err != nil {
		b.logger.Error("Build failed", "error", err)
		return
	}
	b.SaveCaches()
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

	// Create a lightweight parser just for frontmatter check
	md := goldmark.New(goldmark.WithExtensions(meta.Meta))
	context := gParser.NewContext()
	context.Set(mdParser.ContextKeyFilePath, path)
	reader := text.NewReader(source)
	md.Parser().Parse(reader, gParser.WithContext(context))
	metaData := meta.Get(context)
	newFrontmatterHash, _ := utils.GetFrontmatterHash(metaData)

	relPath, _ := utils.SafeRel(b.cfg.ContentDir, path)

	var exists bool
	var cachedHash string

	if b.cacheService != nil {
		if meta, err := b.cacheService.GetPostByPath(relPath); err == nil && meta != nil {
			exists = true
			cachedHash = meta.ContentHash
		}
	}

	if exists && cachedHash == newFrontmatterHash {
		// Content only change: use PostService to process single
		if err := b.postService.ProcessSingle(ctx, path); err != nil {
			b.logger.Error("Failed to process single post", "error", err)
		}
		b.SaveCaches()
	} else {
		// Frontmatter changed or new post: Full rebuild
		if err := b.Build(ctx); err != nil {
			b.logger.Error("Build failed", "error", err)
			return
		}
		b.SaveCaches()
	}
}
