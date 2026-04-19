package generators

import (
	"context"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// DataPagesOptions holds dependencies for data-driven page generation.
type DataPagesOptions struct {
	Ctx    context.Context
	Cfg    *config.Config
	Render models.RenderService
	Data   map[string]any
}

// RenderDataPages iterates through SiteData and renders pages for objects defining a layout and slug.
func RenderDataPages(opts DataPagesOptions) error {
	if opts.Data == nil {
		return nil
	}

	for _, val := range opts.Data {
		// Case 1: Slice of maps (e.g. data/projects.yaml containing a list)
		if list, ok := val.([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					if err := renderDataPageIfEligible(opts, m); err != nil {
						return err
					}
				}
			}
		}

		// Case 2: Single map (e.g. data/about.yaml)
		if m, ok := val.(map[string]any); ok {
			if err := renderDataPageIfEligible(opts, m); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderDataPageIfEligible(opts DataPagesOptions, m map[string]any) error {
	layout := timeutil.ExtractStringFromMap(m, "layout")
	slug := timeutil.ExtractStringFromMap(m, "slug")
	if slug == "" {
		slug = timeutil.ExtractStringFromMap(m, "id")
	}

	if layout != "" && slug != "" {
		title := timeutil.ExtractStringFromMap(m, "title")
		desc := timeutil.ExtractStringFromMap(m, "description")
		
		outputPath := filepath.Join(opts.Cfg.OutputDir, slug+".html")
		
		// Map for templates (frontmatter-like)
		meta := make(map[string]any)
		for k, v := range m {
			meta[k] = v
		}

		return opts.Render.RenderPage(outputPath, models.PageData{
			Title:          title,
			Description:    desc,
			Meta:           meta,
			BaseURL:        opts.Cfg.BaseURL,
			BuildVersion:   opts.Cfg.BuildVersion,
			Config:         opts.Cfg,
			Permalink:      opts.Cfg.BaseURL + "/" + slug + ".html",
			RelativePrefix: "",
			Context:        models.ContextSection,
		})
	}

	return nil
}
