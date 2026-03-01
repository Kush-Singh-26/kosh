package utils

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type PhaseTimer struct {
	name      string
	start     time.Time
	logger    *log.Logger
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
	log.Printf("   ✅ %s completed in %s", p.name, formatDuration(elapsed))
}

func (p *PhaseTimer) StopWithAddendum(addendum string) {
	if p.completed {
		return
	}
	p.completed = true
	elapsed := time.Since(p.start)
	log.Printf("   ✅ %s completed in %s (%s)", p.name, formatDuration(elapsed), addendum)
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
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func FormatDurationShort(d time.Duration) string {
	return formatDuration(d)
}
