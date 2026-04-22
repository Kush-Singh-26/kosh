package parser

import (
	"reflect"
	"testing"
)

func TestExtractD2Hashes(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "No placeholders",
			html: "<div>No diagrams here</div>",
			want: nil,
		},
		{
			name: "Single placeholder",
			html: "<div><!--KOSH_D2:abc123--></div>",
			want: []string{"abc123"},
		},
		{
			name: "Multiple placeholders",
			html: "<div><!--KOSH_D2:abc123--><!--KOSH_D2:def456--></div>",
			want: []string{"abc123", "def456"},
		},
		{
			name: "Duplicate placeholders",
			html: "<div><!--KOSH_D2:abc123--><!--KOSH_D2:abc123--></div>",
			want: []string{"abc123"},
		},
		{
			name: "Placeholder with registry",
			html: "<div><!--KOSH_D2:abc123--><!--KOSH_D2_REG:abc123:light:dark--></div>",
			want: []string{"abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractD2Hashes(tt.html); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractD2Hashes() = %v, want %v", got, tt.want)
			}
		})
	}
}
