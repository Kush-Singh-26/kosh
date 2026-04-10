package post

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/async"
)

func (service *postService) finalizeBuild(processContext *postProcessContext) {
	if len(processContext.newPostsMeta) > 0 && service.cache != nil {
		service.cacheWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    service.logger,
			Operation: "cache commit",
			Fn: func() error {
				return service.cache.BatchCommit(processContext.newPostsMeta, processContext.newSearchRecords, processContext.newDependencies)
			},
			Cleanup: func() {
				service.cacheWg.Done()
			},
		})
	}
}
