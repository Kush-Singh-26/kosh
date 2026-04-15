package post

import (
	"strings"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
)

func TestDateParsingFidelity(t *testing.T) {
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: "content",
			OutputDir:  "public",
		},
	}
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, mdParser.WithDiagramCache(mdParser.NewMemorySSRMap()))
		},
	}

	testCases := []struct {
		name    string
		source  string
		wantDay int
	}{
		{
			name:    "Quoted date (string)",
			source:  "---\ntitle: Quoted\ndate: \"2026-04-09\"\n---\nContent",
			wantDay: 9,
		},
		{
			name:    "Unquoted date (time.Time)",
			source:  "---\ntitle: Unquoted\ndate: 2026-04-09\n---\nContent",
			wantDay: 9,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ParseMarkdownMetadata(ParseOptions{
				Source:           []byte(tc.source),
				Path:             "content/test.md",
				CleanHTMLRelPath: "test.html",
				HTMLRelPath:      "test.html",
				MdPool:           mdPool,
				Cfg:              cfg,
			})
			if err != nil {
				t.Fatalf("ParseMarkdownMetadata failed: %v", err)
			}

			if res.Post.DateObj.IsZero() {
				t.Error("DateObj is zero (Jan 01, 0001)")
			}

			if res.Post.DateObj.Day() != tc.wantDay {
				t.Errorf("Expected day %d, got %d (date: %v)", tc.wantDay, res.Post.DateObj.Day(), res.Post.DateObj)
			}

			wantYear := 2026
			if res.Post.DateObj.Year() != wantYear {
				t.Errorf("Expected year %d, got %d", wantYear, res.Post.DateObj.Year())
			}
		})
	}
}

func TestShortcodeIntegration(t *testing.T) {
	cfg := &config.Config{}
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, mdParser.WithDiagramCache(mdParser.NewMemorySSRMap()))
		},
	}

	// We need to simulate the shortcode processing that happens in worker.go
	// Since ParseMarkdown doesn't take the processor, it takes the ALREADY processed source.
	// This test verifies that if we pass processed source to ParseMarkdown, it renders correctly.

	processedSource := []byte(`---
title: Shortcode Test
---
<div class="shortcode-callout callout-warning">
    
    <div class="callout-content">Beware of bugs</div>
</div>`)

	res, err := ParseMarkdown(ParseOptions{
		Source:           processedSource,
		Path:             "content/test.md",
		CleanHTMLRelPath: "test.html",
		HTMLRelPath:      "test.html",
		MdPool:           mdPool,
		Cfg:              cfg,
		BodyOffset:       24, // length of frontmatter + delimiters
	})

	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	if !strings.Contains(res.HTMLContent, "shortcode-callout") {
		t.Errorf("HTML content missing shortcode output. Got: %s", res.HTMLContent)
	}
	if !strings.Contains(res.HTMLContent, "callout-warning") {
		t.Errorf("HTML content missing callout type. Got: %s", res.HTMLContent)
	}
}
