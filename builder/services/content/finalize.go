package content

import (
	"github.com/Kush-Singh-26/kosh/builder/async"
)

func (service *contentService) finalizeBuild(processContext *contentProcessContext) {
	// The pages were (re-)parsed under the current D2 config, so the HTML
	// cache is valid for it again from here on.
	if service.pendingD2Sig != "" && service.cache != nil {
		if err := service.cache.SetD2Sig(service.pendingD2Sig); err != nil {
			service.logger.Warn("Failed to store D2 signature", "error", err)
		}
		service.pendingD2Sig = ""
	}
	if len(processContext.newItemsMeta) > 0 && service.cache != nil {
		service.cacheWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       service.ctx.Ctx,
			Logger:    service.logger,
			Operation: "cache commit",
			Fn: func() error {
				return service.cache.BatchCommit(processContext.newItemsMeta, processContext.newSearchRecords, processContext.newDependencies)
			},
			Cleanup: func() {
				service.cacheWg.Done()
			},
		})
	}
}
