package buildCtx

import (
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

// BuildContext holds build-scoped state and dependencies to avoid global variables.
type BuildContext struct {
	IsTesting    bool
	IsDev        bool
	IsCleanBuild bool
	Scheduler    scheduler.BuildScheduler
	Logger       *slog.Logger
}

// NewBuildContext creates a new BuildContext.
func NewBuildContext(isTesting, isDev, isClean bool, s scheduler.BuildScheduler, l *slog.Logger) *BuildContext {
	return &BuildContext{
		IsTesting:    isTesting,
		IsDev:        isDev,
		IsCleanBuild: isClean,
		Scheduler:    s,
		Logger:       l,
	}
}

// DetectTestingMode inspects os.Args to determine if we are running in a test context.
// Deprecated: Use fs.DetectTestingMode() directly.
func DetectTestingMode() bool {
	return fs.DetectTestingMode()
}
