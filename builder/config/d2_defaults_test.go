package config

import (
	"strings"
	"testing"
)

// The Nord retint is the default D2 look: diagrams must melt into Nord pages
// (Snow Storm light / Polar Night dark) instead of shipping D2's stock white
// and Dark Mauve backgrounds. Guard the defaults against accidental zeroing.
func TestDefaultD2ConfigIsNord(t *testing.T) {
	cfg := defaultD2Config()

	if cfg.Pad <= 0 {
		t.Errorf("default D2 Pad must be positive (breathing room), got %d", cfg.Pad)
	}
	if cfg.LightOverrides == nil || cfg.DarkOverrides == nil {
		t.Fatal("default D2 theme overrides must be set (Nord retint)")
	}

	sig := cfg.ConfigSignature()
	for _, want := range []string{"#ECEFF4", "#2E3440", "#5E81AC", "#88C0D0"} {
		if !strings.Contains(sig, want) {
			t.Errorf("default D2 signature missing Nord color %s: %s", want, sig)
		}
	}
}
