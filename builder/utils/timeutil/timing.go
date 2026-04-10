package timeutil

import (
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	phaseDirMode   = 0755
	phaseFileMode  = 0644
	unitPerMilli   = 1000.0
	floatPrecision = 2
	floatBitSize   = 64
)

// PhaseTimer measures the duration of a named build phase.
type PhaseTimer struct {
	name      string
	start     time.Time
	completed bool
}

// StartPhase starts timing for a named phase.
func StartPhase(name string) *PhaseTimer {
	return &PhaseTimer{
		name:  name,
		start: time.Now(),
	}
}

// Stop stops the timer and records the phase duration.
func (timer *PhaseTimer) Stop() {
	if timer.completed {
		return
	}
	timer.completed = true
	elapsed := time.Since(timer.start)
	TrackPhase(timer.name, elapsed)
	slog.Info("Phase completed", "name", timer.name, "duration", formatDuration(elapsed))
}

// StopWithAddendum stops the timer and logs with an addendum.
func (timer *PhaseTimer) StopWithAddendum(addendum string) {
	if timer.completed {
		return
	}
	timer.completed = true
	elapsed := time.Since(timer.start)
	TrackPhase(timer.name, elapsed)
	slog.Info("Phase completed", "name", timer.name, "duration", formatDuration(elapsed), "addendum", addendum)
}

// PhaseTracker accumulates named phase durations.
type PhaseTracker struct {
	mu      sync.Mutex // protects phases and enabled
	phases  map[string]time.Duration
	enabled bool
}

var globalTracker = &PhaseTracker{
	phases:  make(map[string]time.Duration),
	enabled: false,
}

// EnablePhaseTracking enables global phase tracking.
func EnablePhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.enabled = true
}

// DisablePhaseTracking disables global phase tracking.
func DisablePhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.enabled = false
}

// ResetPhaseTracking clears all recorded phase durations.
func ResetPhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.phases = make(map[string]time.Duration)
}

// TrackPhase records a phase duration when tracking is enabled.
func TrackPhase(name string, duration time.Duration) {
	if !globalTracker.enabled {
		return
	}
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.phases[name] = duration
}

// GetPhaseDurations returns a snapshot of recorded phase durations.
func GetPhaseDurations() map[string]time.Duration {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	result := make(map[string]time.Duration, len(globalTracker.phases))
	maps.Copy(result, globalTracker.phases)
	return result
}

// IsPhaseTrackingEnabled reports whether phase tracking is enabled.
func IsPhaseTrackingEnabled() bool {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	return globalTracker.enabled
}

// PhaseDurationSnapshot holds a formatted phase duration.
type PhaseDurationSnapshot struct {
	Name         string `json:"name"`
	Milliseconds int64  `json:"milliseconds"`
	Formatted    string `json:"formatted"`
}

// GetSortedPhaseDurations returns phase durations sorted by time descending.
func GetSortedPhaseDurations() []PhaseDurationSnapshot {
	phases := GetPhaseDurations()
	result := make([]PhaseDurationSnapshot, 0, len(phases))
	for name, duration := range phases {
		result = append(result, PhaseDurationSnapshot{
			Name:         name,
			Milliseconds: duration.Milliseconds(),
			Formatted:    FormatDurationShort(duration),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Milliseconds == result[j].Milliseconds {
			return result[i].Name < result[j].Name
		}
		return result[i].Milliseconds > result[j].Milliseconds
	})

	return result
}

// FormatPhaseSummary renders a human-readable phase summary.
func FormatPhaseSummary() string {
	phases := GetSortedPhaseDurations()
	if len(phases) == 0 {
		return ""
	}

	var summary strings.Builder
	summary.WriteString("Phase timings:\n")
	for _, phase := range phases {
		summary.WriteString("  ")
		summary.WriteString(phase.Name)
		summary.WriteString(": ")
		summary.WriteString(phase.Formatted)
		summary.WriteByte('\n')
	}
	return summary.String()
}

// WritePhaseDurationsJSON writes phase durations to a JSON file.
func WritePhaseDurationsJSON(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), phaseDirMode); err != nil {
		return err
	}

	data, err := json.MarshalIndent(struct {
		GeneratedAt string                  `json:"generated_at"`
		Phases      []PhaseDurationSnapshot `json:"phases"`
	}{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Phases:      GetSortedPhaseDurations(),
	}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, phaseFileMode)
}

func formatDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		val := float64(duration.Nanoseconds()) / unitPerMilli
		return strconv.FormatFloat(val, 'f', floatPrecision, floatBitSize) + "us"
	}
	if duration < time.Second {
		val := float64(duration.Microseconds()) / unitPerMilli
		return strconv.FormatFloat(val, 'f', floatPrecision, floatBitSize) + "ms"
	}
	val := duration.Seconds()
	return strconv.FormatFloat(val, 'f', floatPrecision, floatBitSize) + "s"
}

// FormatDurationShort returns a compact duration string.
func FormatDurationShort(duration time.Duration) string {
	return formatDuration(duration)
}
