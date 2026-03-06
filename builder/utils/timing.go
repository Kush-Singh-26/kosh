package utils

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PhaseTimer struct {
	name      string
	start     time.Time
	completed bool
}

func StartPhase(name string) *PhaseTimer {
	return &PhaseTimer{
		name:  name,
		start: time.Now(),
	}
}

func (p *PhaseTimer) Stop() {
	if p.completed {
		return
	}
	p.completed = true
	elapsed := time.Since(p.start)
	TrackPhase(p.name, elapsed)
	slog.Info("Phase completed", "name", p.name, "duration", formatDuration(elapsed))
}

func (p *PhaseTimer) StopWithAddendum(addendum string) {
	if p.completed {
		return
	}
	p.completed = true
	elapsed := time.Since(p.start)
	TrackPhase(p.name, elapsed)
	slog.Info("Phase completed", "name", p.name, "duration", formatDuration(elapsed), "addendum", addendum)
}

type PhaseTracker struct {
	mu      sync.Mutex
	phases  map[string]time.Duration
	enabled bool
}

var globalTracker = &PhaseTracker{
	phases:  make(map[string]time.Duration),
	enabled: false,
}

func EnablePhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.enabled = true
}

func DisablePhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.enabled = false
}

func ResetPhaseTracking() {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.phases = make(map[string]time.Duration)
}

func TrackPhase(name string, duration time.Duration) {
	if !globalTracker.enabled {
		return
	}
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	globalTracker.phases[name] = duration
}

func GetPhaseDurations() map[string]time.Duration {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	result := make(map[string]time.Duration, len(globalTracker.phases))
	for k, v := range globalTracker.phases {
		result[k] = v
	}
	return result
}

func IsPhaseTrackingEnabled() bool {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()
	return globalTracker.enabled
}

type PhaseDurationSnapshot struct {
	Name         string `json:"name"`
	Milliseconds int64  `json:"milliseconds"`
	Formatted    string `json:"formatted"`
}

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

func FormatPhaseSummary() string {
	phases := GetSortedPhaseDurations()
	if len(phases) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Phase timings:\n")
	for _, phase := range phases {
		sb.WriteString("  ")
		sb.WriteString(phase.Name)
		sb.WriteString(": ")
		sb.WriteString(phase.Formatted)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func WritePhaseDurationsJSON(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
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

	return os.WriteFile(path, data, 0644)
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		val := float64(d.Nanoseconds()) / 1000.0
		return strconv.FormatFloat(val, 'f', 2, 64) + "us"
	}
	if d < time.Second {
		val := float64(d.Microseconds()) / 1000.0
		return strconv.FormatFloat(val, 'f', 2, 64) + "ms"
	}
	val := d.Seconds()
	return strconv.FormatFloat(val, 'f', 2, 64) + "s"
}

func FormatDurationShort(d time.Duration) string {
	return formatDuration(d)
}
