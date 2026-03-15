package services

import (
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func TestAssetService_Build(t *testing.T) {
	// We need real OS FS because esbuild runs as external process
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	cacheDir := filepath.Join(tmpDir, "cache")

	_ = os.MkdirAll(filepath.Join(sourceDir, "css"), 0755)
	_ = os.MkdirAll(outputDir, 0755)
	_ = os.MkdirAll(cacheDir, 0755)

	cssContent := `
body {
    background-color: white;
    color: black;
}
`
	cssPath := filepath.Join(sourceDir, "css", "main.css")
	if err := os.WriteFile(cssPath, []byte(cssContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSS: %v", err)
	}

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			StaticDir: sourceDir,
			OutputDir: outputDir,
			CacheDir:  cacheDir,
		},
		BuildOptions: config.BuildOptions{
			CompressImages: true,
		},
	}

	sourceFs := afero.NewOsFs()
	sink := testutil.NewMemSink()
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewAssetService(sourceFs, sink, cfg, mockRend, logger)

	ctx := context.Background()
	if err := svc.Build(ctx); err != nil {
		t.Fatalf("Asset Build failed: %v", err)
	}

	// Verify that CSS was processed and written to sink
	// Note: esbuild adds hash when minification/production settings are used
	// or it might use absolute paths based on our sink call
	foundCSS := false
	for path := range sink.Files {
		if strings.Contains(path, "main") && strings.HasSuffix(path, ".css") {
			foundCSS = true
			break
		}
	}

	if !foundCSS {
		t.Errorf("CSS not found in sink. Available: %v", getSinkKeys(sink))
	}
}

func getSinkKeys(sink *testutil.MemSink) []string {
	keys := make([]string, 0, len(sink.Files))
	for k := range sink.Files {
		keys = append(keys, k)
	}
	return keys
}

func TestAssetService_Build_DoesNotHardlinkSourceStaticFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	cacheDir := filepath.Join(tmpDir, "cache")

	staticDir := filepath.Join(sourceDir, "static")
	sourceLogo := filepath.Join(staticDir, "images", "logo.png")
	outputLogo := filepath.Join(outputDir, "static", "images", "logo.png")

	if err := os.MkdirAll(filepath.Dir(sourceLogo), 0755); err != nil {
		t.Fatalf("failed to create source image dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputLogo), 0755); err != nil {
		t.Fatalf("failed to create output image dir: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	original := []byte("PNG_SOURCE_BYTES")
	if err := os.WriteFile(sourceLogo, original, 0644); err != nil {
		t.Fatalf("failed to write source logo: %v", err)
	}

	// Pre-create output file with same size and mtime as source so old hardlink path
	// would have linked source->output and allowed source mutation via output writes.
	if err := os.WriteFile(outputLogo, original, 0644); err != nil {
		t.Fatalf("failed to write output logo: %v", err)
	}
	srcInfo, err := os.Stat(sourceLogo)
	if err != nil {
		t.Fatalf("failed to stat source logo: %v", err)
	}
	if err := os.Chtimes(outputLogo, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		t.Fatalf("failed to align output mtime: %v", err)
	}

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			StaticDir:  staticDir,
			OutputDir:  outputDir,
			ContentDir: filepath.Join(sourceDir, "content"),
			CacheDir:   cacheDir,
		},
		BuildOptions: config.BuildOptions{
			CompressImages: false,
		},
	}

	if err := os.MkdirAll(cfg.ContentDir, 0755); err != nil {
		t.Fatalf("failed to create content dir: %v", err)
	}

	sourceFs := afero.NewOsFs()
	sink := utils.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewAssetService(sourceFs, sink, cfg, mockRend, logger)
	ctx := context.Background()
	if err := svc.Build(ctx); err != nil {
		t.Fatalf("Asset Build failed: %v", err)
	}

	// Mutate output and ensure source bytes remain unchanged.
	mutated := []byte("MUTATED_OUTPUT")
	if err := os.WriteFile(outputLogo, mutated, 0644); err != nil {
		t.Fatalf("failed to mutate output logo: %v", err)
	}

	sourceAfter, err := os.ReadFile(sourceLogo)
	if err != nil {
		t.Fatalf("failed to read source logo after output mutation: %v", err)
	}
	if !bytes.Equal(sourceAfter, original) {
		t.Fatalf("source static file was modified via output write; got %q want %q", string(sourceAfter), string(original))
	}
}

func TestAssetService_Build_DoesNotCopySourceSearchWasm(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	cacheDir := filepath.Join(tmpDir, "cache")

	wasmDir := filepath.Join(sourceDir, "wasm")
	if err := os.MkdirAll(wasmDir, 0755); err != nil {
		t.Fatalf("failed to create wasm dir: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	staleWasm := []byte("stale search wasm")
	if err := os.WriteFile(filepath.Join(wasmDir, "search.wasm"), staleWasm, 0644); err != nil {
		t.Fatalf("failed to write stale search wasm: %v", err)
	}

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			StaticDir: sourceDir,
			OutputDir: outputDir,
			CacheDir:  cacheDir,
		},
		BuildOptions: config.BuildOptions{
			CompressImages: false,
		},
	}

	sourceFs := afero.NewOsFs()
	sink := utils.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewAssetService(sourceFs, sink, cfg, mockRend, logger)
	if err := svc.Build(context.Background()); err != nil {
		t.Fatalf("asset build failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "static", "wasm", "search.wasm")); err == nil {
		t.Fatalf("source static/wasm/search.wasm should not be copied into output")
	}
}
