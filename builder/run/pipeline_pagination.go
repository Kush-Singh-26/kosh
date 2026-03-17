package run

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

func (b *Builder) renderPagination(ctx context.Context, allPosts, pinnedPosts []models.PostMetadata, force bool) error {
	return generators.RenderPagination(generators.PaginationOptions{
		Ctx:         ctx,
		Cfg:         b.cfg,
		Sink:        b.Sink,
		Render:      b.deps.Render,
		Cache:       b.deps.Cache,
		SourceFs:    b.SourceFs,
		AllPosts:    allPosts,
		PinnedPosts: pinnedPosts,
		Force:       force,
		Logger:      b.logger,
		FaviconPath: b.getFaviconPath(),
	})
}

func (b *Builder) renderTags(ctx context.Context, tagMap map[string][]models.PostMetadata, forceSocialRebuild bool) error {
	return generators.RenderTags(generators.TagOptions{
		Ctx:                ctx,
		Cfg:                b.cfg,
		Sink:               b.Sink,
		Render:             b.deps.Render,
		Cache:              b.deps.Cache,
		SourceFs:           b.SourceFs,
		TagMap:             tagMap,
		ForceSocialRebuild: forceSocialRebuild,
		FaviconPath:        b.getFaviconPath(),
	})
}

func (b *Builder) provideSocialCard(destPath string, cacheKey string, title, description, badge string, force bool) {
	generators.ProvideSocialCard(generators.ProvideSocialCardOptions{
		Sink:        b.Sink,
		Cache:       b.deps.Cache,
		SourceFs:    b.SourceFs,
		OutputDir:   b.cfg.OutputDir,
		CacheDir:    b.cfg.CacheDir,
		Title:       b.cfg.Title,
		DestPath:    destPath,
		CacheKey:    cacheKey,
		CardTitle:   title,
		Description: description,
		Badge:       badge,
		Force:       force,
		SocialCfg:   &b.cfg.SocialCards,
		Render:      b.deps.Render,
		FaviconPath: b.getFaviconPath(),
	})
}
