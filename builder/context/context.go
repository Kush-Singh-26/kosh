package buildCtx

import (
	"log/slog"
	"os"
	"strings"

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
func DetectTestingMode() bool {
	if len(os.Args) > 0 {
		if strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe") || strings.Contains(os.Args[0], "_test") {
			return true
		}
	}
	return false
}
