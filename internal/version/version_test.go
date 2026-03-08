package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
)

func TestRunFs_VersionTransition(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Initial setup
	koshYaml := `
title: "Test Blog"
versions:
  - name: "v1.0"
    path: ""
    isLatest: true
`
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(koshYaml), 0644)
	_ = fs.MkdirAll("content", 0755)
	_ = afero.WriteFile(fs, "content/hello.md", []byte("hello"), 0644)

	// Run transition to v2.0
	RunFs(fs, []string{"v2.0"})

	// Verify kosh.yaml was updated
	cfg := loadConfigFs(fs)
	if len(cfg.Versions) != 2 {
		t.Fatalf("Expected 2 versions, got %d", len(cfg.Versions))
	}

	if cfg.Versions[0].Name != "v2.0" || !cfg.Versions[0].IsLatest || cfg.Versions[0].Path != "" {
		t.Errorf("New version config incorrect: %+v", cfg.Versions[0])
	}

	if cfg.Versions[1].Name != "v1.0" || cfg.Versions[1].IsLatest || cfg.Versions[1].Path != "v1.0" {
		t.Errorf("Old version config incorrect: %+v", cfg.Versions[1])
	}

	// Verify content was frozen
	exists, _ := afero.Exists(fs, "content/v1.0/hello.md")
	if !exists {
		t.Error("Content was not frozen to content/v1.0/")
	}
}

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

func TestCopyFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.md")
	dst := filepath.Join(tmpDir, "dst.md")
	content := []byte("hello world")

	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("copyFile() content = %q, want %q", string(got), string(content))
	}
}

func TestSnapshotContent(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change wd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Setup source
	srcDir := "content"
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "post.md"), []byte("post"), 0644); err != nil {
		t.Fatalf("Failed to create post: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "v1.0"), 0755); err != nil {
		t.Fatalf("Failed to create v1.0 dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "v1.0", "old.md"), []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old.md: %v", err)
	}

	// Setup config
	cfg := &config.Config{
		Versions: []config.Version{
			{Name: "v1.0", Path: "v1.0", IsLatest: false},
			{Name: "v2.0", Path: "", IsLatest: true},
		},
	}

	destDir := filepath.Join(srcDir, "v2.0")
	if err := snapshotContent(destDir, srcDir, cfg); err != nil {
		t.Fatalf("snapshotContent() failed: %v", err)
	}

	// Verify
	if _, err := os.Stat(filepath.Join(destDir, "post.md")); err != nil {
		t.Errorf("Snapshot should contain post.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "v1.0")); err == nil {
		t.Error("Snapshot should NOT contain other version directories")
	}
}
