package clean

import (
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
)

func TestRunFs_CleanAll(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create Project structure
	// We need to match what config.LoadFs returns
	cfg := config.LoadFs(fs, []string{})
	outputDir := cfg.OutputDir
	cacheDir := cfg.CacheDir

	_ = fs.MkdirAll(filepath.Join(outputDir, "v1"), 0755)
	_ = afero.WriteFile(fs, filepath.Join(outputDir, "index.html"), []byte("test"), 0644)
	_ = fs.MkdirAll(cacheDir, 0755)

	RunFs(fs, true, true)

	exists, _ := afero.DirExists(fs, outputDir)
	if exists {
		t.Errorf("output directory %s should have been cleaned", outputDir)
	}

	exists, _ = afero.DirExists(fs, cacheDir)
	if exists {
		t.Errorf("cache directory %s should have been cleaned", cacheDir)
	}
}
