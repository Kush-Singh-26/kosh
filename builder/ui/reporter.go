package ui

import (
	"time"
)

// Phase represents a distinct part of the build process.
type Phase int

const (
	// PhaseScan represents the metadata scan phase.
	PhaseScan Phase = iota
	// PhaseAssets represents asset build/copy.
	PhaseAssets
	// PhasePosts represents markdown parsing and rendering.
	PhasePosts
	// PhaseSiteWide represents site-wide generators.
	PhaseSiteWide
	// PhasePublish represents publish/commit of outputs.
	PhasePublish
	// PhaseIncremental represents incremental rebuilds.
	PhaseIncremental
)

// String returns the display label for a Phase.
func (p Phase) String() string {
	switch p {
	case PhaseScan:
		return "Metadata Scan"
	case PhaseAssets:
		return "Building Assets"
	case PhasePosts:
		return "Processing Posts"
	case PhaseSiteWide:
		return "Site-wide Rendering"
	case PhasePublish:
		return "Publishing"
	case PhaseIncremental:
		return "Incremental Rebuild"
	default:
		return "Unknown Phase"
	}
}

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
