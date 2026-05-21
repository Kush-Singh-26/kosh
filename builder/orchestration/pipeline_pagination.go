package orchestration

import (
	"context"
	"html/template"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type renderPaginationOptions struct {
	workingContext context.Context
	allPosts       []models.ContentMetadata
	pinnedItems    []models.ContentMetadata
	force          bool
	taxonomies     map[string]models.TaxonomyData
	showcaseMath   template.HTML
	showcaseD2     template.HTML
}

func (engineInstance *Engine) renderPagination(options renderPaginationOptions) error {
	workingContext := options.workingContext
	allPosts := options.allPosts
	pinnedItems := options.pinnedItems
	force := options.force
	taxonomies := options.taxonomies

	return generators.RenderPagination(generators.PaginationOptions{
		ShowcaseMath: options.showcaseMath,
		ShowcaseD2:   options.showcaseD2,
		Ctx:          workingContext,
		Cfg:          engineInstance.Cfg,
		Sink:         engineInstance.artifactSink,
		Render:       engineInstance.Deps.Render,
		Cache:        engineInstance.Deps.Cache,
		SourceFs:     engineInstance.Deps.SourceFs,
		AllPosts:     allPosts,
		PinnedItems:  pinnedItems,
		Force:        force,
		Logger:       engineInstance.Deps.Logger,
		LogoPath:     engineInstance.GetLogoPath(),
		Taxonomies:   taxonomies,
	})
}

func (engineInstance *Engine) renderTaxonomies(workingContext context.Context, taxonomyMap map[string]map[string][]models.ContentMetadata, forceSocialRebuild bool) error {
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
