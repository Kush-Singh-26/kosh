package post

import (
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
			return mdParser.New(cfg, nil, mdParser.NewMemorySSRMap(), nil)
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
				CleanHtmlRelPath: "test.html",
				HtmlRelPath:      "test.html",
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
