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

	RunFs(fs, true, true, true)

	exists, _ := afero.DirExists(fs, outputDir)
	if exists {
		t.Errorf("output directory %s should have been cleaned", outputDir)
	}

	exists, _ = afero.DirExists(fs, cacheDir)
	if exists {
		t.Errorf("cache directory %s should have been cleaned", cacheDir)
	}
}

func TestRunFs_CleanRootOnly(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("public/v1", 0755)
	_ = afero.WriteFile(fs, "public/index.html", []byte("test"), 0644)

	// Create a dummy kosh.yaml to load config
	koshYaml := `
outputDir: "public"
versions:
  - name: "v1"
    path: "v1"
`
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(koshYaml), 0644)

	// Since config.Load reads from OS, we might need to mock config loading
	// or ensure config.Load uses afero as well.
	// For now, let's see if we can manually pass config if we refactor more.
	// But let's try to use the existing structure.

	// If I can't easily mock config.Load, I'll just test the logic with a helper.

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			OutputDir: "public",
		},
		Versions: []config.Version{
			{Name: "v1", Path: "v1"},
		},
	}

	cleanRootFilesOnly(fs, "public", cfg, true)

	exists, _ := afero.Exists(fs, "public/index.html")
	if exists {
		t.Error("public/index.html should have been cleaned")
	}

	exists, _ = afero.DirExists(fs, "public/v1")
	if !exists {
		t.Error("public/v1 should have been preserved")
	}
}
