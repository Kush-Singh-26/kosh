package version

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
)

func TestFindLatestVersion(t *testing.T) {
	tests := []struct {
		name      string
		versions  []config.Version
		wantIdx   int
		wantName  string
		wantFound bool
	}{
		{
			name: "find latest version",
			versions: []config.Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: false},
				{Name: "v2.0", Path: "v2.0", IsLatest: false},
				{Name: "v3.0", Path: "", IsLatest: true},
			},
			wantIdx:   2,
			wantName:  "v3.0",
			wantFound: true,
		},
		{
			name: "find latest version with index",
			versions: []config.Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: false},
				{Name: "v2.0", Path: "", IsLatest: true},
				{Name: "v3.0", Path: "v3.0", IsLatest: false},
			},
			wantIdx:   1,
			wantName:  "v2.0",
			wantFound: true,
		},
		{
			name: "no latest version",
			versions: []config.Version{
				{Name: "v1.0", Path: "v1.0", IsLatest: false},
				{Name: "v2.0", Path: "v2.0", IsLatest: false},
			},
			wantIdx:   -1,
			wantName:  "",
			wantFound: false,
		},
		{
			name:      "empty versions",
			versions:  []config.Version{},
			wantIdx:   -1,
			wantName:  "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Versions: tt.versions}
			idx, got := findLatestVersion(cfg)
			if tt.wantFound {
				if got == nil {
					t.Errorf("findLatestVersion() = nil, want name %q", tt.wantName)
				} else if idx != tt.wantIdx {
					t.Errorf("findLatestVersion() idx = %d, want %d", idx, tt.wantIdx)
				} else if got.Name != tt.wantName {
					t.Errorf("findLatestVersion() name = %v, want %q", got.Name, tt.wantName)
				}
			} else {
				if got != nil {
					t.Errorf("findLatestVersion() = %v, want nil", got)
				}
				if idx != tt.wantIdx {
					t.Errorf("findLatestVersion() idx = %d, want %d", idx, tt.wantIdx)
				}
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Run("nonexistent source", func(t *testing.T) {
		err := copyFile("nonexistent_source_file_12345.md", "nonexistent_dest.md")
		if err == nil {
			t.Error("copyFile() should return error for nonexistent source")
		}
	})
}
