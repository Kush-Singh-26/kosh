package orchestration

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type renderPaginationOptions struct {
	workingContext context.Context
	allPosts       []models.PostMetadata
	pinnedPosts    []models.PostMetadata
	force          bool
	taxonomies     map[string]models.TaxonomyData
}

func (engineInstance *Engine) renderPagination(options renderPaginationOptions) error {
	workingContext := options.workingContext
	allPosts := options.allPosts
	pinnedPosts := options.pinnedPosts
	force := options.force
	taxonomies := options.taxonomies

	return generators.RenderPagination(generators.PaginationOptions{
		Ctx:         workingContext,
		Cfg:         engineInstance.Cfg,
		Sink:        engineInstance.artifactSink,
		Render:      engineInstance.Deps.Render,
		Cache:       engineInstance.Deps.Cache,
		SourceFs:    engineInstance.Deps.SourceFs,
		AllPosts:    allPosts,
		PinnedPosts: pinnedPosts,
		Force:       force,
		Logger:      engineInstance.Deps.Logger,
		LogoPath:    engineInstance.GetLogoPath(),
		Taxonomies:  taxonomies,
	})
}

func (engineInstance *Engine) renderTaxonomies(workingContext context.Context, taxonomyMap map[string]map[string][]models.PostMetadata, forceSocialRebuild bool) error {
	return generators.RenderTaxonomies(generators.TaxonomyOptions{
		Ctx:                workingContext,
		Cfg:                engineInstance.Cfg,
		Sink:               engineInstance.artifactSink,
		Render:             engineInstance.Deps.Render,
		Cache:              engineInstance.Deps.Cache,
		SourceFs:           engineInstance.Deps.SourceFs,
		TaxonomyMap:        taxonomyMap,
		ForceSocialRebuild: forceSocialRebuild,
		LogoPath:           engineInstance.GetLogoPath(),
	})
}
