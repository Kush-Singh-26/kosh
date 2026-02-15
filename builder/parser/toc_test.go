package parser

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestTOCTransformer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name: "basic headings",
			input: `
# Title (Level 1 ignored)
## Heading 2
### Heading 3
#### Heading 4
`,
			expected: 3,
		},
		{
			name: "headings with attributes",
			input: `
## Heading 2 {#custom-id}
### Heading 3
`,
			expected: 2,
		},
		{
			name:     "no headings",
			input:    "Just some text",
			expected: 0,
		},
		{
			name: "nested headings",
			input: `
## H2
### H3
## H2 again
`,
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := goldmark.New(
				goldmark.WithParserOptions(
					parser.WithASTTransformers(
						util.Prioritized(&tocTransformer{}, 100),
					),
					parser.WithAutoHeadingID(),
				),
			)

			context := parser.NewContext()
			reader := text.NewReader([]byte(tt.input))
			md.Parser().Parse(reader, parser.WithContext(context))

			toc := GetTOC(context)
			if len(toc) != tt.expected {
				t.Errorf("expected %d TOC entries, got %d", tt.expected, len(toc))
			}

			// Verify levels logic
			for _, entry := range toc {
				if entry.Level < 2 || entry.Level > 6 {
					t.Errorf("TOC entry level %d out of range [2,6]", entry.Level)
				}
				if entry.ID == "" {
					t.Error("TOC entry has empty ID")
				}
				if entry.Text == "" {
					t.Error("TOC entry has empty text")
				}
			}
		})
	}
}

func TestGetTOCNil(t *testing.T) {
	// Test safe behavior when key is missing
	context := parser.NewContext()
	toc := GetTOC(context)
	if toc != nil {
		t.Error("GetTOC should return nil when key is missing")
	}
}
