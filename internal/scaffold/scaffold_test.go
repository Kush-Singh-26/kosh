package scaffold

import (
	"testing"

	"github.com/spf13/afero"
)

func TestRunFs(t *testing.T) {
	fs := afero.NewMemMapFs()
	RunFs(fs, []string{})

	// Verify directories
	dirs := []string{"content", "themes", "public", "static"}
	for _, dir := range dirs {
		exists, _ := afero.DirExists(fs, dir)
		if !exists {
			t.Errorf("Directory %s was not created", dir)
		}
	}

	// Verify kosh.yaml
	exists, _ := afero.Exists(fs, "kosh.yaml")
	if !exists {
		t.Error("kosh.yaml was not created")
	}

	// Verify first post
	exists, _ = afero.Exists(fs, "content/hello-world.md")
	if !exists {
		t.Error("content/hello-world.md was not created")
	}
}
