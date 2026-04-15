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
func (level HealthLevel) String() string {
	switch level {
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
func (registry *BuildHealthRegistry) Reset() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.events = nil
	registry.phaseStack = nil
	registry.warnings.Store(0)
	registry.errors.Store(0)
	registry.retries.Store(0)
	registry.rollbacks.Store(0)
	registry.slowPhases.Store(0)
	registry.criticalEvents.Store(0)
	registry.searchDocs.Store(0)
	registry.searchSize.Store(0)
	registry.startTime = time.Now()
}

// AddEvent records a health event at the given level.
func (registry *BuildHealthRegistry) AddEvent(level HealthLevel, message string) {
	event := HealthEvent{Level: level, Message: message}
	registry.recordEvent(event)
}

// AddWarning records a warning-level event.
func (registry *BuildHealthRegistry) AddWarning(message string) {
	event := HealthEvent{Level: HealthLevelWarning, Message: message}
	if len(registry.phaseStack) > 0 {
		event.Phase = registry.phaseStack[len(registry.phaseStack)-1]
	}
	registry.warnings.Add(1)
	registry.recordEvent(event)
}

// AddError records an error-level event.
func (registry *BuildHealthRegistry) AddError(message string) {
	event := HealthEvent{Level: HealthLevelError, Message: message}
	if len(registry.phaseStack) > 0 {
		event.Phase = registry.phaseStack[len(registry.phaseStack)-1]
	}
	registry.errors.Add(1)
	registry.recordEvent(event)
}

// AddCritical records a critical-level event.
func (registry *BuildHealthRegistry) AddCritical(message string) {
	event := HealthEvent{Level: HealthLevelCritical, Message: message}
	if len(registry.phaseStack) > 0 {
		event.Phase = registry.phaseStack[len(registry.phaseStack)-1]
	}
	registry.criticalEvents.Add(1)
	registry.recordEvent(event)
}

// RecordRetry records a retry event and updates counters.
func (registry *BuildHealthRegistry) RecordRetry(message string, count int) {
	event := HealthEvent{
		Level:      HealthLevelInfo,
		Message:    message,
		RetryCount: count,
	}
	if len(registry.phaseStack) > 0 {
		event.Phase = registry.phaseStack[len(registry.phaseStack)-1]
	}
	registry.retries.Add(int64(count))
	registry.recordEvent(event)
}

// RecordRollback records a rollback event.
func (registry *BuildHealthRegistry) RecordRollback(message string) {
	event := HealthEvent{Level: HealthLevelWarning, Message: message}
	if len(registry.phaseStack) > 0 {
		event.Phase = registry.phaseStack[len(registry.phaseStack)-1]
	}
	registry.rollbacks.Add(1)
	registry.recordEvent(event)
}

// RecordSearchStats stores the latest search index metrics.
func (registry *BuildHealthRegistry) RecordSearchStats(docs int64, size int64) {
	registry.searchDocs.Store(docs)
	registry.searchSize.Store(size)
}

// RecordSlowPhase records a slow phase event with timing information.
func (registry *BuildHealthRegistry) RecordSlowPhase(phase string, duration time.Duration) {
	threshold := slowPhaseThreshold
	event := HealthEvent{
		Level:    HealthLevelWarning,
		Message:  "Phase exceeded slow threshold",
		Phase:    phase,
		Duration: duration.String(),
	}
	registry.slowPhases.Add(1)
	registry.recordEvent(event)
	if slog.Default().Enabled(context.TODO(), slog.LevelWarn) {
		slog.Warn("Slow phase detected",
			"phase", phase,
			"duration", duration.String(),
			"threshold", threshold.String(),
			"health", "slow_phase")
	}
}

// PushPhase pushes a phase name onto the active phase stack.
func (registry *BuildHealthRegistry) PushPhase(name string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.phaseStack = append(registry.phaseStack, name)
}

// PopPhase removes the most recent phase name if it matches.
func (registry *BuildHealthRegistry) PopPhase(name string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.phaseStack) > 0 && registry.phaseStack[len(registry.phaseStack)-1] == name {
		registry.phaseStack = registry.phaseStack[:len(registry.phaseStack)-1]
	}
}

func (registry *BuildHealthRegistry) recordEvent(event HealthEvent) {
	registry.mu.Lock()
	registry.events = append(registry.events, event)
	registry.mu.Unlock()

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
func (registry *BuildHealthRegistry) Report() BuildHealthReport {
	totalDuration := time.Since(registry.startTime)
	warnings := registry.warnings.Load()
	errors := registry.errors.Load()
	critical := registry.criticalEvents.Load()
	retries := registry.retries.Load()
	rollbacks := registry.rollbacks.Load()
	slowPhases := registry.slowPhases.Load()
	searchDocs := registry.searchDocs.Load()
	searchSize := registry.searchSize.Load()

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
	switch {
	case healthScore < healthLevelCriticalThreshold:
		healthLevel = "critical"
	case healthScore < healthLevelDegradedThreshold:
		healthLevel = "degraded"
	case healthScore < healthLevelHealthyMax:
		healthLevel = "healthy_with_warnings"
	}

	registry.mu.Lock()
	eventCount := len(registry.events)
	registry.mu.Unlock()

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
func (registry *BuildHealthRegistry) LogSummary() {
	report := registry.Report()

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
