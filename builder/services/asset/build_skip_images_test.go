//go:build !wasm

package asset

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type imageReadGuardFs struct {
	afero.Fs
	path  string
	reads atomic.Int32
	deny  bool
}

func (fs *imageReadGuardFs) Open(path string) (afero.File, error) {
	if filepath.Clean(path) == fs.path {
		fs.reads.Add(1)
		if fs.deny {
			return nil, errors.New("unexpected source image decode")
		}
	}
	return fs.Fs.Open(path)
}

func TestBuildSkipImagesCacheMissFallback(t *testing.T) {
	for _, mode := range []string{"static-miss", "content-miss", "disk-hit", "dev-existing", "invalid-miss"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			staticDir := filepath.Join(root, "static")
			contentDir := filepath.Join(root, "content")
			outputDir := filepath.Join(root, "public")
			stagingDir := filepath.Join(root, "staging")
			for _, dir := range []string{staticDir, contentDir} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			relPath := filepath.Join("static", "photo.png")
			imagePath := filepath.Join(staticDir, "photo.png")
			if mode == "content-miss" {
				relPath = "photo.png"
				imagePath = filepath.Join(contentDir, relPath)
			}
			var source bytes.Buffer
			if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			if mode == "invalid-miss" {
				source.Reset()
				source.WriteString("not an image")
			}
			if err := os.WriteFile(imagePath, source.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{
				StaticDir: staticDir, ContentDir: contentDir, OutputDir: outputDir,
				CacheDir: filepath.Join(root, "cache"), SiteRoot: root,
				ShouldCompressImages: true, ImageWorkers: 1,
			}
			guard := &imageReadGuardFs{Fs: afero.NewOsFs(), path: imagePath}
			newService := func(sink fspkg.ArtifactSink) Service {
				svc := NewService(Dependencies{
					SourceFs: guard, Sink: sink, Cfg: cfg,
					Renderer: mocks.NewMockRenderService(), Logger: slog.Default(),
				})
				if mode == "content-miss" {
					info, err := os.Stat(imagePath)
					if err != nil {
						t.Fatal(err)
					}
					ch := make(chan []models.ScannedAsset, 1)
					ch <- []models.ScannedAsset{{Path: imagePath, Info: info}}
					close(ch)
					svc.SetContentAssetsChannel(ch)
				}
				return svc
			}
			// Stop before TempDir cleanup so queued cache writes cannot recreate it.
			t.Cleanup(assets.StopImageCacheWriter)
			if mode == "disk-hit" {
				warmDir := filepath.Join(root, "warm")
				if err := newService(fspkg.NewDiskSink(warmDir, outputDir)).Build(t.Context()); err != nil {
					t.Fatal(err)
				}
				assets.StopImageCacheWriter()
				guard.reads.Store(0)
				guard.deny = true
			}
			if mode == "dev-existing" {
				cfg.IsDev = true
				stagingDir = outputDir
				guard.deny = true
			}
			sink := fspkg.NewDiskSink(stagingDir, outputDir)
			webpRel := relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".webp"
			webpPath := filepath.Join(outputDir, webpRel)
			if mode == "dev-existing" {
				if err := sink.WriteFile(webpPath, []byte("existing WebP")); err != nil {
					t.Fatal(err)
				}
			}
			err := newService(sink).BuildWithOptions(t.Context(), true)
			if mode == "invalid-miss" {
				if err == nil {
					t.Fatal("invalid cache-miss image was silently dropped")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(stagingDir, webpRel))
			if err != nil {
				t.Fatalf("missing WebP in output: %v", err)
			}
			if mode == "dev-existing" {
				if string(data) != "existing WebP" {
					t.Fatal("existing dev image was replaced")
				}
			} else if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
				t.Fatal("output is not a WebP image")
			}
			if (mode == "disk-hit" || mode == "dev-existing") && guard.reads.Load() != 0 {
				t.Fatal("cache hit decoded the source image")
			}
			if !sink.GetWrittenFiles()[webpPath] {
				t.Fatal("WebP was not registered for orphan protection")
			}
			if err := os.WriteFile(filepath.Join(stagingDir, webpRel), []byte("mutated output"), 0644); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(imagePath)
			if err != nil || !bytes.Equal(after, source.Bytes()) {
				t.Fatalf("source image was mutated: %v", err)
			}
			if mode != "dev-existing" {
				if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
					t.Fatalf("build wrote outside staging: %v", err)
				}
			}
		})
	}
}
