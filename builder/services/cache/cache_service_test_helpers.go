package cache

import (
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func setupCacheServiceTest(t *testing.T) (*cacheService, *cache.Manager, func()) {
	t.Helper()

	mgr, cleanup := testutil.CreateTestCache(t)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	service := NewService(Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Manager: mgr,
		Logger:  logger,
	}).(*cacheService)
	return service, mgr, cleanup
}
