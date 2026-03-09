package parser

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

func TestAdmonitions(t *testing.T) {
	cfg := &config.Config{}
	r := native.New()
	diagramCache := &sync.Map{}
	d2Group := r.GetD2Singleflight()
	md := New(cfg, r, diagramCache, d2Group)

	tests := []struct {
		name     string
		source   string
		contains []string
	}{
		{
			name: "note admonition",
			source: `!!! note
This is a note.
!!!`,
			contains: []string{`class="admonition adm-note"`, "This is a note."},
		},
		{
			name: "warning admonition",
			source: `!!! warning
Be careful!
!!!`,
			contains: []string{`class="admonition adm-warning"`, "Be careful!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := md.Convert([]byte(tt.source), &buf); err != nil {
				t.Fatalf("Markdown conversion failed: %v", err)
			}

			got := buf.String()
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("Result does not contain %q\nGot: %s", c, got)
				}
			}
		})
	}
}
