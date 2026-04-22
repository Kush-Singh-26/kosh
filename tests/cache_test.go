package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestCacheConsistency(t *testing.T) {
	// 1. Setup paths
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(mockSiteDir, "content"), 0755)
	require.NoError(t, err)

	koshYaml := `
title: "Cache Site"
baseURL: "https://example.com"
theme: "blog"
themeDir: "../../themes"
outputDir: "public"
staticDir: "static"
contentDir: "content"
`
	err = os.WriteFile(filepath.Join(mockSiteDir, "kosh.yaml"), []byte(koshYaml), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(mockSiteDir, "content", "post.md"), []byte("---\ntitle: Post\ndate: 2026-04-14\n---\nBody"), 0644)
	require.NoError(t, err)

	err = os.Chdir(mockSiteDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(wd) }()

	cfg := config.Load(nil)
	cfg.BuildVersion = 1234567890
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.ThemeDir = filepath.Join(repoRoot, "tests", "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	// 2. First Build (Cold)
	cfg.OutputDir = filepath.Join(mockSiteDir, "public-cold")
	sourceFs := afero.NewOsFs()
	reporter := &mockReporter{}
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)
	// Force cold build by ensuring cache dir is empty (it should be already)
	err = engine.Build(context.Background())
	engine.Close() // Close BoltDB so next engine can open it
	require.NoError(t, err)

	hashCold := hashDirectory(t, cfg.OutputDir)

	// 3. Second Build (Warm)
	cfg2 := *cfg
	cfg2.OutputDir = filepath.Join(mockSiteDir, "public-warm")
	// Re-initialize engine to simulate a fresh process using the same cache
	engine2 := orchestration.NewEngine(
		orchestration.WithConfig(&cfg2),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)
	err = engine2.Build(context.Background())
	engine2.Close()
	require.NoError(t, err)

	hashWarm := hashDirectory(t, cfg2.OutputDir)

	// Compare hashes
	for file, coldHash := range hashCold {
		warmHash, ok := hashWarm[file]
		require.True(t, ok, "File %s missing in warm build", file)
		if coldHash != warmHash {
			coldContent, _ := os.ReadFile(filepath.Join(cfg.OutputDir, file))
			warmContent, _ := os.ReadFile(filepath.Join(cfg2.OutputDir, file))
			t.Errorf("File %s changed in warm build.\nCold content hash: %s\nWarm content hash: %s\n\nCold:\n%s\n\nWarm:\n%s", file, coldHash, warmHash, string(coldContent), string(warmContent))
		}
		require.Equal(t, coldHash, warmHash, "File %s changed in warm build", file)
	}
	require.Equal(t, len(hashCold), len(hashWarm), "Different number of files in cold and warm builds")
}

func hashDirectory(t *testing.T, dir string) map[string]string {
	hashes := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Skip files that naturally change (like timestamps in RSS/Sitemap if any)
		// search.bin is excluded from bit-identical check due to non-deterministic map iteration in msgp
		if filepath.Ext(path) == ".xml" || filepath.Base(path) == "manifest.json" || filepath.Base(path) == "graph.json" || filepath.Base(path) == "search.bin" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		hashes[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	require.NoError(t, err)
	return hashes
}
