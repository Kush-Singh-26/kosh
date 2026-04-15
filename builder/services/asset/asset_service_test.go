//go:build !wasm

package asset

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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
	cssPath := filepath.Join(sourceDir, "css", "layout.css")
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
			ShouldCompressImages: false,
		},
	}

	sourceFs := afero.NewOsFs()
	sink := testutil.NewMemSink()
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewService(Dependencies{
		SourceFs: sourceFs,
		Sink:     sink,
		Cfg:      cfg,
		Renderer: mockRend,
		Logger:   logger,
	})

	ctx := context.Background()
	if err := svc.Build(ctx); err != nil {
		t.Fatalf("Asset Build failed: %v", err)
	}

	// Verify that CSS was processed and written to sink
	// Note: esbuild adds hash when minification/production settings are used
	// or it might use absolute paths based on our sink call
	foundCSS := false
	for path := range sink.Files {
		if strings.Contains(path, "layout") && strings.HasSuffix(path, ".css") {
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
			ShouldCompressImages: false,
		},
	}

	if err := os.MkdirAll(cfg.ContentDir, 0755); err != nil {
		t.Fatalf("failed to create content dir: %v", err)
	}

	sourceFs := afero.NewOsFs()
	sink := fspkg.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewService(Dependencies{
		SourceFs: sourceFs,
		Sink:     sink,
		Cfg:      cfg,
		Renderer: mockRend,
		Logger:   logger,
	})
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
			ShouldCompressImages: false,
		},
	}

	sourceFs := afero.NewOsFs()
	sink := fspkg.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewService(Dependencies{
		SourceFs: sourceFs,
		Sink:     sink,
		Cfg:      cfg,
		Renderer: mockRend,
		Logger:   logger,
	})
	if err := svc.Build(context.Background()); err != nil {
		t.Fatalf("asset build failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "static", "wasm", "search.wasm")); err == nil {
		t.Fatalf("source static/wasm/search.wasm should not be copied into output")
	}
}

func TestAssetService_Build_ContextCancellationRace(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	cacheDir := filepath.Join(tmpDir, "cache")

	staticDir := filepath.Join(sourceDir, "static")
	if err := os.MkdirAll(filepath.Join(staticDir, "images"), 0755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	// Create enough files to ensure ParallelWalk takes some time and uses multiple workers
	const numFiles = 100
	for i := 0; i < numFiles; i++ {
		path := filepath.Join(staticDir, "images", "img"+strconv.Itoa(i)+".png")
		if err := os.WriteFile(path, []byte("IMAGE_DATA"), 0644); err != nil {
			t.Fatalf("failed to write test image: %v", err)
		}
	}

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			StaticDir: sourceDir,
			OutputDir: outputDir,
			CacheDir:  cacheDir,
		},
		SiteRoot: sourceDir,
		BuildOptions: config.BuildOptions{
			ShouldCompressImages: false,
		},
	}
	sourceFs := afero.NewOsFs()
	sink := fspkg.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewService(Dependencies{
		SourceFs: sourceFs,
		Sink:     sink,
		Cfg:      cfg,
		Renderer: mockRend,
		Logger:   logger,
	})

	// Simulate fast scanner return by passing a closed channel
	closedChan := make(chan []models.ScannedAsset)
	close(closedChan)
	svc.SetContentAssetsChannel(closedChan)

	ctx := context.Background()
	if err := svc.Build(ctx); err != nil {
		t.Fatalf("Asset Build failed: %v", err)
	}

	// Verify all files were discovered and enqueued
	count := 0
	_ = filepath.Walk(filepath.Join(outputDir, "static", "images"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".png") {
			count++
		}
		return nil
	})

	if count != numFiles {
		t.Errorf("Race condition detected: expected %d images to be processed, got %d", numFiles, count)
	}
}

func TestAssetService_Build_ImageCompression(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	cacheDir := filepath.Join(tmpDir, "cache")

	staticDir := filepath.Join(sourceDir, "static")
	if err := os.MkdirAll(filepath.Join(staticDir, "images"), 0755); err != nil {
		t.Fatalf("failed to create static dir: %v", err)
	}

	imagePath := filepath.Join(staticDir, "images", "test.png")
	// Create a real 1x1 PNG image
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255}) // Red pixel
	f, err := os.Create(imagePath)
	if err != nil {
		t.Fatalf("failed to create image file: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("failed to encode png: %v", err)
	}
	_ = f.Close() // best-effort; write already succeeded

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			StaticDir: sourceDir,
			OutputDir: outputDir,
			CacheDir:  cacheDir,
		},
		SiteRoot: sourceDir,
		BuildOptions: config.BuildOptions{
			ShouldCompressImages: true,
			WebPQuality:          80,
		},
	}
	sourceFs := afero.NewOsFs()
	sink := fspkg.NewDiskSink(outputDir, outputDir)
	mockRend := mocks.NewMockRenderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svc := NewService(Dependencies{
		SourceFs: sourceFs,
		Sink:     sink,
		Cfg:      cfg,
		Renderer: mockRend,
		Logger:   logger,
	})

	ctx := context.Background()
	if err := svc.Build(ctx); err != nil {
		t.Fatalf("Asset Build failed: %v", err)
	}

	// Verify .webp exists and original .png is gone after cleanup
	webpPath := filepath.Join(outputDir, "static", "images", "test.webp")
	if _, err := os.Stat(webpPath); os.IsNotExist(err) {
		t.Errorf("Expected WebP image to be generated at %s", webpPath)
	}

	// Manually trigger cleanup as orchestration would do
	assets.CleanupOriginalImages(outputDir)

	originalOutputPath := filepath.Join(outputDir, "static", "images", "test.png")
	if _, err := os.Stat(originalOutputPath); err == nil {
		t.Errorf("Original image %s should have been cleaned up", originalOutputPath)
	}
}
