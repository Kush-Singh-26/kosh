package metrics

import (
	"strings"
	"testing"
)

func TestPanicsRecovered(t *testing.T) {
	m := NewBuildMetrics()

	// Initial string should not contain panic warning
	if strings.Contains(m.String(), "panic(s) recovered") {
		t.Error("Panic warning should not be present initially")
	}

	m.IncrementPanicsRecovered()
	m.IncrementPanicsRecovered()

	// Metric value should increment atomically
	if m.PanicsRecovered != 2 {
		t.Errorf("Expected 2 panics, got %d", m.PanicsRecovered)
	}

	// String should now contain tracking text
	output := m.String()
	if !strings.Contains(output, "2 panic(s) recovered during build") {
		t.Errorf("Output misses panic tracking: %s", output)
	}
}

func TestImageResizeSkippedMetric(t *testing.T) {
	m := NewBuildMetrics()
	m.RecordImageOptimization(1000, 600)
	m.RecordImageResizeSkipped()
	m.RecordImageResizeSkipped()

	out := m.String()
	if !strings.Contains(out, "Skipped resize for 2 small images") {
		t.Fatalf("missing skipped resize metric in output: %s", out)
	}
}
