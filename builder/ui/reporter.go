package ui

import (
	"time"
)

// Phase represents a distinct part of the build process.
type Phase string

const (
	PhaseScan        Phase = "Metadata Scan"
	PhaseAssets      Phase = "Building Assets"
	PhasePosts       Phase = "Processing Posts"
	PhaseSiteWide    Phase = "Site-wide Rendering"
	PhasePublish     Phase = "Publishing"
	PhaseIncremental Phase = "Incremental Rebuild"
)

// Reporter handles all build-time terminal reporting.
type Reporter interface {
	Start(mode string)
	StartPhase(phase Phase)
	UpdateProgress(phase Phase, current, total int, detail string)
	EndPhase(phase Phase, duration time.Duration)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, err error, args ...any)
	Success(msg string)
	Status(msg string)
	Finish(stats BuildStats)
}

// BuildStats holds final build metrics for the reporter.
type BuildStats struct {
	Duration   time.Duration
	HitRate    float64
	Posts      int
	Assets     int
	Optimized  int
	SavedBytes int64
}
