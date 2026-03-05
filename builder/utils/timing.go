package utils

import (
	"log/slog"
	"strconv"
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
	slog.Info("Phase completed", "name", p.name, "duration", formatDuration(elapsed))
}

func (p *PhaseTimer) StopWithAddendum(addendum string) {
	if p.completed {
		return
	}
	p.completed = true
	elapsed := time.Since(p.start)
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
	globalTracker.enabled = true
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

func formatDuration(d time.Duration) string {
	// Avoid fmt.Sprintf to prevent data races with global fmt buffer
	if d < time.Millisecond {
		// Format as microseconds with 2 decimal places
		val := float64(d.Microseconds()) / 1000.0
		return strconv.FormatFloat(val, 'f', 2, 64) + "µs"
	}
	if d < time.Second {
		// Format as milliseconds with 2 decimal places
		val := float64(d.Milliseconds()) / 1000.0
		return strconv.FormatFloat(val, 'f', 2, 64) + "ms"
	}
	// Format as seconds with 2 decimal places
	val := d.Seconds()
	return strconv.FormatFloat(val, 'f', 2, 64) + "s"
}

func FormatDurationShort(d time.Duration) string {
	return formatDuration(d)
}
