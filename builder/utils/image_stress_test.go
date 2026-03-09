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
	for i := 0; i < 20; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		for x := 0; x < 100; x++ {
			for y := 0; y < 100; y++ {
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
	err := CopyDirVFS(ctx, srcFs, sink, srcDir, destDir, true, []string{}, func(s string) {}, cacheDir, 8, 80, nil)
	if err != nil {
		t.Fatalf("CopyDirVFS failed: %v", err)
	}

	// Verify all images were converted to webp
	for i := 0; i < 20; i++ {
		webpPath := filepath.Join(destDir, fmt.Sprintf("img%d.webp", i))
		if _, err := os.Stat(webpPath); os.IsNotExist(err) {
			t.Errorf("Expected webp image %s not found", webpPath)
		}
	}
}
