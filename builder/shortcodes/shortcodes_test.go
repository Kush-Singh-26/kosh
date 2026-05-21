package shortcodes

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestShortcodes(t *testing.T) {
	fs := afero.NewMemMapFs()
	processor, err := New(fs, "")
	if err != nil {
		t.Fatalf("Failed to create shortcode processor: %v", err)
	}

	tests := []struct {
		name     string
		markdown string
		contains []string
	}{
		{
			name:     "YouTube Shortcode",
			markdown: `{{< youtube id="dQw4w9WgXcQ" >}}`,
			contains: []string{`src="https://www.youtube.com/embed/dQw4w9WgXcQ"`, `class="shortcode-youtube"`},
		},
		{
			name:     "Figure Shortcode",
			markdown: `{{< figure src="/img/test.png" caption="Test Image" >}}`,
			contains: []string{`<figure class="shortcode-figure`, `src="/img/test.png"`, `<figcaption>Test Image</figcaption>`},
		},
		{
			name:     "Callout Shortcode (Default)",
			markdown: `{{< callout >}}This is a note{{< /callout >}}`,
			contains: []string{`class="admonition admonition-note"`, `This is a note`},
		},
		{
			name:     "Callout Shortcode (Warning)",
			markdown: `{{< callout type="warning" title="Warning Title" >}}Be careful!{{< /callout >}}`,
			contains: []string{`class="admonition admonition-warning"`, `class="admonition-title">Warning Title`, `Be careful!`},
		},
		{
			name:     "Details Shortcode",
			markdown: `{{< details summary="Click for more" >}}Hidden content{{< /details >}}`,
			contains: []string{`<details class="shortcode-details"`, `<summary>Click for more</summary>`, `Hidden content`},
		},
		{
			name:     "Nested/Mixed Shortcodes",
			markdown: `{{< callout >}}{{< youtube id="123" >}}{{< /callout >}}`,
			contains: []string{`class="admonition`, `src="https://www.youtube.com/embed/123"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processor.Process([]byte(tt.markdown))
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			output := string(got)
			for _, c := range tt.contains {
				if !strings.Contains(output, c) {
					t.Errorf("Result does not contain %q\nGot: %s", c, output)
				}
			}
		})
	}
}

func TestMismatchedTags(t *testing.T) {
	fs := afero.NewMemMapFs()
	processor, err := New(fs, "")
	if err != nil {
		t.Fatalf("Failed to create shortcode processor: %v", err)
	}

	markdown := `{{< callout >}}content{{< /youtube >}}`
	got, err := processor.Process([]byte(markdown))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(got) != markdown {
		t.Errorf("Expected mismatched tags to be ignored, got: %s", string(got))
	}
}
