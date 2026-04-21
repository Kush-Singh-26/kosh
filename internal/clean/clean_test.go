package clean

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
)

func TestRunFs_CleanAll(t *testing.T) {
	cfg := config.Load([]string{})
	outputDir := cfg.OutputDir
	cacheDir := cfg.CacheDir

	tmpDir := t.TempDir()

	outputPath := filepath.Join(tmpDir, outputDir)
	cachePath := filepath.Join(tmpDir, cacheDir)

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCwd)

	if err := os.MkdirAll(filepath.Join(outputPath, "v1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputPath, "index.html"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		t.Fatal(err)
	}

	if err := RunFs(context.Background(), nil, []string{}, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Errorf("output directory %s should have been cleaned", outputDir)
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cache directory %s should have been cleaned", cacheDir)
	}
}