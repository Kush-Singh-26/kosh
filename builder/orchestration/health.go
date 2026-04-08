package orchestration

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	slowPhaseThreshold           = 5 * time.Second
	healthScoreStart             = 100
	healthScoreMin               = 0
	rollbackPenalty              = 10
	criticalPenalty              = 25
	errorPenalty                 = 5
	slowPhasePenalty             = 2
	healthLevelCriticalThreshold = 50
	healthLevelDegradedThreshold = 75
	healthLevelHealthyMax        = 100
	bytesPerKiB                  = 1024
)

// HealthLevel describes the severity of a build health event.
type HealthLevel int

const (
	// HealthLevelInfo represents informational events.
	HealthLevelInfo HealthLevel = 0
	// HealthLevelWarning represents recoverable warnings.
	HealthLevelWarning HealthLevel = 1
	// HealthLevelError represents error-level events.
	HealthLevelError HealthLevel = 2
	// HealthLevelCritical represents critical failures.
	HealthLevelCritical HealthLevel = 3
)

// String returns the string label for the health level.
func (h HealthLevel) String() string {
	switch h {
	case HealthLevelInfo:
		return "info"
	case HealthLevelWarning:
		return "warning"
	case HealthLevelError:
		return "error"
	case HealthLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// HealthEvent records a single health event with optional context.
type HealthEvent struct {
	Level      HealthLevel `json:"level"`
	Message    string      `json:"message"`
	Phase      string      `json:"phase,omitempty"`
	Duration   string      `json:"duration,omitempty"`
	RetryCount int         `json:"retry_count,omitempty"`
	Path       string      `json:"path,omitempty"`
}

// BuildHealthRegistry collects and reports build health events and metrics.
type BuildHealthRegistry struct {
	mu         sync.Mutex // protects events and phaseStack
	events     []HealthEvent
	phaseStack []string

	// Aggregate counters
	warnings       atomic.Int64
	errors         atomic.Int64
	retries        atomic.Int64
	rollbacks      atomic.Int64
	slowPhases     atomic.Int64
	criticalEvents atomic.Int64

	// Search metrics
	searchDocs atomic.Int64
	searchSize atomic.Int64 // in bytes

	// Timing
	startTime time.Time
}

// NewBuildHealthRegistry returns a new build health registry.
func NewBuildHealthRegistry() *BuildHealthRegistry {
	return &BuildHealthRegistry{
		startTime: time.Now(),
	}
}

// Reset clears all recorded events and counters.
func (r *BuildHealthRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
	r.phaseStack = nil
	r.warnings.Store(0)
	r.errors.Store(0)
	r.retries.Store(0)
	r.rollbacks.Store(0)
	r.slowPhases.Store(0)
	r.criticalEvents.Store(0)
	r.searchDocs.Store(0)
	r.searchSize.Store(0)
	r.startTime = time.Now()
}

// AddEvent records a health event at the given level.
func (r *BuildHealthRegistry) AddEvent(level HealthLevel, message string) {
	event := HealthEvent{Level: level, Message: message}
	r.recordEvent(event)
}

// AddWarning records a warning-level event.
func (r *BuildHealthRegistry) AddWarning(msg string) {
	event := HealthEvent{Level: HealthLevelWarning, Message: msg}
	if len(r.phaseStack) > 0 {
		event.Phase = r.phaseStack[len(r.phaseStack)-1]
	}
	r.warnings.Add(1)
	r.recordEvent(event)
}

// AddError records an error-level event.
func (r *BuildHealthRegistry) AddError(msg string) {
	event := HealthEvent{Level: HealthLevelError, Message: msg}
	if len(r.phaseStack) > 0 {
		event.Phase = r.phaseStack[len(r.phaseStack)-1]
	}
	r.errors.Add(1)
	r.recordEvent(event)
}

// AddCritical records a critical-level event.
func (r *BuildHealthRegistry) AddCritical(msg string) {
	event := HealthEvent{Level: HealthLevelCritical, Message: msg}
	if len(r.phaseStack) > 0 {
		event.Phase = r.phaseStack[len(r.phaseStack)-1]
	}
	r.criticalEvents.Add(1)
	r.recordEvent(event)
}

// RecordRetry records a retry event and updates counters.
func (r *BuildHealthRegistry) RecordRetry(msg string, count int) {
	event := HealthEvent{
		Level:      HealthLevelInfo,
		Message:    msg,
		RetryCount: count,
	}
	if len(r.phaseStack) > 0 {
		event.Phase = r.phaseStack[len(r.phaseStack)-1]
	}
	r.retries.Add(int64(count))
	r.recordEvent(event)
}

// RecordRollback records a rollback event.
func (r *BuildHealthRegistry) RecordRollback(msg string) {
	event := HealthEvent{Level: HealthLevelWarning, Message: msg}
	if len(r.phaseStack) > 0 {
		event.Phase = r.phaseStack[len(r.phaseStack)-1]
	}
	r.rollbacks.Add(1)
	r.recordEvent(event)
}

// RecordSearchStats stores the latest search index metrics.
func (r *BuildHealthRegistry) RecordSearchStats(docs int64, size int64) {
	r.searchDocs.Store(docs)
	r.searchSize.Store(size)
}

// RecordSlowPhase records a slow phase event with timing information.
func (r *BuildHealthRegistry) RecordSlowPhase(phase string, duration time.Duration) {
	threshold := slowPhaseThreshold
	event := HealthEvent{
		Level:    HealthLevelWarning,
		Message:  "Phase exceeded slow threshold",
		Phase:    phase,
		Duration: duration.String(),
	}
	r.slowPhases.Add(1)
	r.recordEvent(event)
	if slog.Default().Enabled(context.TODO(), slog.LevelWarn) {
		slog.Warn("Slow phase detected",
			"phase", phase,
			"duration", duration.String(),
			"threshold", threshold.String(),
			"health", "slow_phase")
	}
}

// PushPhase pushes a phase name onto the active phase stack.
func (r *BuildHealthRegistry) PushPhase(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phaseStack = append(r.phaseStack, name)
}

// PopPhase removes the most recent phase name if it matches.
func (r *BuildHealthRegistry) PopPhase(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.phaseStack) > 0 && r.phaseStack[len(r.phaseStack)-1] == name {
		r.phaseStack = r.phaseStack[:len(r.phaseStack)-1]
	}
}

func (r *BuildHealthRegistry) recordEvent(event HealthEvent) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()

	attrs := []any{
		"health_level", event.Level.String(),
		"message", event.Message,
	}
	if event.Phase != "" {
		attrs = append(attrs, "phase", event.Phase)
	}
	if event.Duration != "" {
		attrs = append(attrs, "duration", event.Duration)
	}
	if event.RetryCount > 0 {
		attrs = append(attrs, "retry_count", event.RetryCount)
	}
	if event.Path != "" {
		attrs = append(attrs, "path", event.Path)
	}

	switch event.Level {
	case HealthLevelInfo:
		slog.Debug("BuildHealth", attrs...)
	case HealthLevelWarning:
		slog.Warn("BuildHealth", attrs...)
	case HealthLevelError, HealthLevelCritical:
		slog.Error("BuildHealth", attrs...)
	}
}

// BuildHealthReport summarizes health metrics for a build.
type BuildHealthReport struct {
	TotalDuration  time.Duration `json:"total_duration"`
	Warnings       int64         `json:"warnings"`
	Errors         int64         `json:"errors"`
	CriticalEvents int64         `json:"critical_events"`
	Retries        int64         `json:"retries"`
	Rollbacks      int64         `json:"rollbacks"`
	SlowPhases     int64         `json:"slow_phases"`
	HealthScore    int           `json:"health_score"`
	HealthLevel    string        `json:"health_level"`
	EventCount     int           `json:"event_count"`
	SearchDocs     int64         `json:"search_docs"`
	SearchSize     int64         `json:"search_size"`
}

// Report builds a summary report for the current registry state.
func (r *BuildHealthRegistry) Report() BuildHealthReport {
	totalDuration := time.Since(r.startTime)
	warnings := r.warnings.Load()
	errors := r.errors.Load()
	critical := r.criticalEvents.Load()
	retries := r.retries.Load()
	rollbacks := r.rollbacks.Load()
	slowPhases := r.slowPhases.Load()
	searchDocs := r.searchDocs.Load()
	searchSize := r.searchSize.Load()

	healthScore := healthScoreStart
	if rollbacks > 0 {
		healthScore -= int(rollbacks) * rollbackPenalty
	}
	if critical > 0 {
		healthScore -= int(critical) * criticalPenalty
	}
	if errors > 0 {
		healthScore -= int(errors) * errorPenalty
	}
	if slowPhases > 0 {
		healthScore -= int(slowPhases) * slowPhasePenalty
	}
	if healthScore < healthScoreMin {
		healthScore = healthScoreMin
	}

	healthLevel := "healthy"
	if healthScore < healthLevelCriticalThreshold {
		healthLevel = "critical"
	} else if healthScore < healthLevelDegradedThreshold {
		healthLevel = "degraded"
	} else if healthScore < healthLevelHealthyMax {
		healthLevel = "healthy_with_warnings"
	}

	r.mu.Lock()
	eventCount := len(r.events)
	r.mu.Unlock()

	return BuildHealthReport{
		TotalDuration:  totalDuration,
		Warnings:       warnings,
		Errors:         errors,
		CriticalEvents: critical,
		Retries:        retries,
		Rollbacks:      rollbacks,
		SlowPhases:     slowPhases,
		HealthScore:    healthScore,
		HealthLevel:    healthLevel,
		EventCount:     eventCount,
		SearchDocs:     searchDocs,
		SearchSize:     searchSize,
	}
}

// LogSummary logs a summary of build health metrics.
func (r *BuildHealthRegistry) LogSummary() {
	report := r.Report()

	if slog.Default().Enabled(context.TODO(), slog.LevelInfo) {
		slog.Info("Build health report",
			"duration", report.TotalDuration.String(),
			"warnings", report.Warnings,
			"errors", report.Errors,
			"rollbacks", report.Rollbacks,
			"slow_phases", report.SlowPhases,
			"health_score", report.HealthScore,
			"health_level", report.HealthLevel,
			"search_docs", report.SearchDocs,
			"search_size_kb", report.SearchSize/bytesPerKiB)
	}

	if report.HealthLevel == "critical" {
		slog.Error("Build health CRITICAL - manual inspection recommended",
			"health_score", report.HealthScore,
			"critical_events", report.CriticalEvents,
			"errors", report.Errors,
			"rollbacks", report.Rollbacks)
	}
}
