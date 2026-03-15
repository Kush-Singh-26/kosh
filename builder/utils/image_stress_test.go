package utils

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestImageOptimizationStress(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	destDir := filepath.Join(tmpDir, "dest")
	cacheDir := filepath.Join(tmpDir, "cache")

	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(destDir, 0755)
	_ = os.MkdirAll(cacheDir, 0755)

	// Generate 20 dummy PNG images
	for i := range 20 {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		for x := range 100 {
			for y := range 100 {
				img.Set(x, y, color.RGBA{uint8(i * 10), uint8(x * 2), uint8(y * 2), 255})
			}
		}
		f, _ := os.Create(filepath.Join(srcDir, fmt.Sprintf("img%d.png", i)))
		_ = png.Encode(f, img)
		_ = f.Close()
	}

	srcFs := afero.NewOsFs()
	sink := NewDiskSink(destDir, destDir)

	ctx := context.Background()

	// Process images in parallel
	opts := CopyOptions{
		Compress:     true,
		ExcludeExts:  []string{},
		OnWrite:      func(s string) {},
		CacheDir:     cacheDir,
		ImageWorkers: 8,
		WebPQuality:  80,
		Metrics:      nil,
	}
	err := CopyDirVFS(ctx, srcFs, sink, srcDir, destDir, opts)
	if err != nil {
		t.Fatalf("CopyDirVFS failed: %v", err)
	}

	// Verify all images were converted to webp
	for i := range 20 {
		webpPath := filepath.Join(destDir, fmt.Sprintf("img%d.webp", i))
		if _, err := os.Stat(webpPath); os.IsNotExist(err) {
			t.Errorf("Expected webp image %s not found", webpPath)
		}
	}
}
