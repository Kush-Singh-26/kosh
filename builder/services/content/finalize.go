package content

import (
	"github.com/Kush-Singh-26/kosh/builder/async"
)

func (service *contentService) finalizeBuild(processContext *contentProcessContext) {
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
