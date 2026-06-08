package orchestration

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// waitForSiteWideRendering waits for site-wide generators and renders 404 if needed.
func (engineInstance *Engine) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *timeutil.PhaseTimer, siteWideHas404 bool, metadataContext *content.Context) error {
	if siteWideGroup == nil {
		return nil
	}

	if err := siteWideGroup.Wait(); err != nil {
		if siteTimer != nil {
			siteTimer.Stop()
		}
		return err
	}
	if siteTimer != nil {
		siteTimer.Stop()
	}
	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhaseSiteWide, 0)
		engineInstance.Deps.Reporter.StartPhase(ui.PhasePublish)
	}

	if siteWideHas404 {
		var taxonomies map[string]models.TaxonomyData
		if metadataContext != nil {
			taxonomies = metadataContext.Taxonomies
		}
		if err := engineInstance.Deps.Render.Render404(filepath.Join(engineInstance.Cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: engineInstance.Cfg.BaseURL, TabTitle: "404 Not Found",
			Config: engineInstance.Cfg, RelativePrefix: "/", Taxonomies: taxonomies, Context: models.ContextHome,
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}
