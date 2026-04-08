package orchestration

import (
	"fmt"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"golang.org/x/sync/errgroup"
)

// waitForSiteWideRendering waits for site-wide generators and renders 404 if needed.
func (b *Engine) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *timeutil.PhaseTimer, siteWideHas404 bool) error {
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
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhaseSiteWide, 0)
		b.Deps.Reporter.StartPhase(ui.PhasePublish)
	}

	if siteWideHas404 {
		if err := b.Deps.Render.Render404(filepath.Join(b.Cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: b.Cfg.BaseURL, TabTitle: "404 Not Found",
			Config: b.Cfg, RelativePrefix: "",
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}
