package timeutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPhaseTrackingLifecycle(t *testing.T) {
	ResetPhaseTracking()
	EnablePhaseTracking()
	defer DisablePhaseTracking()

	TrackPhase("render", 150*time.Millisecond)
	TrackPhase("assets", 50*time.Millisecond)

	phases := GetSortedPhaseDurations()
	if len(phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(phases))
	}
	if phases[0].Name != "render" {
		t.Fatalf("expected render to be first, got %s", phases[0].Name)
	}

	summary := FormatPhaseSummary()
	if !strings.Contains(summary, "render") {
		t.Fatalf("expected summary to include render, got %q", summary)
	}
}

func TestWritePhaseDurationsJSON(t *testing.T) {
	ResetPhaseTracking()
	EnablePhaseTracking()
	defer DisablePhaseTracking()

	TrackPhase("search", 20*time.Millisecond)

	outputPath := filepath.Join(t.TempDir(), "phases.json")
	if err := WritePhaseDurationsJSON(outputPath); err != nil {
		t.Fatalf("WritePhaseDurationsJSON failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read phase output: %v", err)
	}
	if !strings.Contains(string(data), "\"search\"") {
		t.Fatalf("expected search phase in JSON output, got %q", string(data))
	}
}
