package post

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/async"
)

func (s *postService) finalizeBuild(pc *postProcessContext) {
	if len(pc.newPostsMeta) > 0 && s.cache != nil {
		s.cacheWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    s.logger,
			Operation: "cache commit",
			Fn: func() error {
				return s.cache.BatchCommit(pc.newPostsMeta, pc.newSearchRecords, pc.newDeps)
			},
			Cleanup: func() {
				s.cacheWg.Done()
			},
		})
	}
}
