package buildCtx

import (
	"log/slog"

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

type ContextOptions struct {
	IsTesting    bool
	IsDev        bool
	IsCleanBuild bool
	Scheduler    scheduler.BuildScheduler
	Logger       *slog.Logger
}

// NewBuildContext creates a new BuildContext.
func NewBuildContext(opts ContextOptions) *BuildContext {
	return &BuildContext{
		IsTesting:    opts.IsTesting,
		IsDev:        opts.IsDev,
		IsCleanBuild: opts.IsCleanBuild,
		Scheduler:    opts.Scheduler,
		Logger:       opts.Logger,
	}
}
