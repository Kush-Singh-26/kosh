package buildctx

import (
	"context"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

// BuildContext holds build-scoped state and dependencies to avoid global variables.
type BuildContext struct {
	Ctx          context.Context
	Cancel       context.CancelFunc
	IsTesting    bool
	IsDev        bool
	IsCleanBuild bool
	Scheduler    scheduler.BuildScheduler
	Logger       *slog.Logger
}

// ContextOptions configures NewBuildContext.
type ContextOptions struct {
	Ctx          context.Context
	IsTesting    bool
	IsDev        bool
	IsCleanBuild bool
	Scheduler    scheduler.BuildScheduler
	Logger       *slog.Logger
}

// NewBuildContext creates a new BuildContext.
func NewBuildContext(opts ContextOptions) *BuildContext {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	return &BuildContext{
		Ctx:          ctx,
		Cancel:       cancel,
		IsTesting:    opts.IsTesting,
		IsDev:        opts.IsDev,
		IsCleanBuild: opts.IsCleanBuild,
		Scheduler:    opts.Scheduler,
		Logger:       opts.Logger,
	}
}
