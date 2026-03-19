package orchestration

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

func (b *Engine) renderPagination(ctx context.Context, allPosts, pinnedPosts []models.PostMetadata, force bool) error {
	return generators.RenderPagination(generators.PaginationOptions{
		Ctx:         ctx,
		Cfg:         b.Cfg,
		Sink:        b.Sink,
		Render:      b.Deps.Render,
		Cache:       b.Deps.Cache,
		SourceFs:    b.SourceFs,
		AllPosts:    allPosts,
		PinnedPosts: pinnedPosts,
		Force:       force,
		Logger:      b.Logger,
		FaviconPath: b.getFaviconPath(),
	})
}

func (b *Engine) renderTags(ctx context.Context, tagMap map[string][]models.PostMetadata, forceSocialRebuild bool) error {
	return generators.RenderTags(generators.TagOptions{
		Ctx:                ctx,
		Cfg:                b.Cfg,
		Sink:               b.Sink,
		Render:             b.Deps.Render,
		Cache:              b.Deps.Cache,
		SourceFs:           b.SourceFs,
		TagMap:             tagMap,
		ForceSocialRebuild: forceSocialRebuild,
		FaviconPath:        b.getFaviconPath(),
	})
}

func (b *Engine) provideSocialCard(destPath string, cacheKey string, title, description, badge string, force bool) {
	generators.ProvideSocialCard(generators.ProvideSocialCardOptions{
		Sink:        b.Sink,
		Cache:       b.Deps.Cache,
		SourceFs:    b.SourceFs,
		OutputDir:   b.Cfg.OutputDir,
		CacheDir:    b.Cfg.CacheDir,
		Title:       b.Cfg.Title,
		DestPath:    destPath,
		CacheKey:    cacheKey,
		CardTitle:   title,
		Description: description,
		Badge:       badge,
		Force:       force,
		SocialCfg:   &b.Cfg.SocialCards,
		Render:      b.Deps.Render,
		FaviconPath: b.getFaviconPath(),
	})
}
