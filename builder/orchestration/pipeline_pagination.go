package orchestration

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

func (b *Engine) renderPagination(ctx context.Context, allPosts, pinnedPosts []models.PostMetadata, force bool, allTags []models.TagData) error {
	return generators.RenderPagination(generators.PaginationOptions{
		Ctx:         ctx,
		Cfg:         b.Cfg,
		Sink:        b.Sink,
		Render:      b.Deps.Render,
		Cache:       b.Deps.Cache,
		SourceFs:    b.Deps.SourceFs,
		AllPosts:    allPosts,
		PinnedPosts: pinnedPosts,
		Force:       force,
		Logger:      b.Deps.Logger,
		LogoPath:    b.getLogoPath(),
		AllTags:     allTags,
	})
}

func (b *Engine) renderTags(ctx context.Context, tagMap map[string][]models.PostMetadata, forceSocialRebuild bool) error {
	return generators.RenderTags(generators.TagOptions{
		Ctx:                ctx,
		Cfg:                b.Cfg,
		Sink:               b.Sink,
		Render:             b.Deps.Render,
		Cache:              b.Deps.Cache,
		SourceFs:           b.Deps.SourceFs,
		TagMap:             tagMap,
		ForceSocialRebuild: forceSocialRebuild,
		LogoPath:           b.getLogoPath(),
	})
}
