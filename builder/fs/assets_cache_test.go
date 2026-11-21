package fs

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

type assetCacheHitScheduler struct{}

func (assetCacheHitScheduler) Acquire(context.Context, scheduler.TaskType) error {
	return errors.New("unexpected esbuild rebuild on cache hit")
}

func (assetCacheHitScheduler) Release(scheduler.TaskType) {}

func writeAssetTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func assetTestSnapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}

func assetCacheFixture(t *testing.T, minify bool) (BuildAssetsOptions, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "source", "static")
	for name, data := range map[string]string{
		"css/main.css":     `@font-face{font-family:test;src:url("../fonts/test.woff2")}body{font-family:test}`,
		"fonts/test.woff2": "test font bytes",
		"lib/shared.js":    `export const shared = () => console.log("shared output");`,
		"js/first.js":      `import {shared} from "../lib/shared.js"; shared();`,
		"js/second.js":     `import {shared} from "../lib/shared.js"; shared();`,
	} {
		writeAssetTestFile(t, filepath.Join(src, filepath.FromSlash(name)), []byte(data))
	}
	out := filepath.Join(root, "public")
	return BuildAssetsOptions{
		SrcFs: afero.NewOsFs(), SrcDir: src, DestDir: filepath.Join(out, "static"),
		Sink:     NewDiskSink(filepath.Join(root, "cold"), out),
		CacheDir: filepath.Join(root, "cache"), Minify: minify,
	}, root
}

func TestAssetCacheWarmRestoresAllOutputs(t *testing.T) {
	for _, minify := range []bool{false, true} {
		name := "development"
		if minify {
			name = "production"
		}
		t.Run(name, func(t *testing.T) {
			opts, root := assetCacheFixture(t, minify)
			sourceBefore := assetTestSnapshot(t, opts.SrcDir)
			coldMap, err := BuildAssetsEsbuild(opts)
			if err != nil {
				t.Fatal(err)
			}
			coldFiles := assetTestSnapshot(t, filepath.Join(root, "cold", "static"))
			var hasFont, hasChunk, hasMap bool
			for name := range coldFiles {
				hasFont = hasFont || strings.HasSuffix(name, ".woff2")
				hasChunk = hasChunk || strings.HasPrefix(name, "chunk-") && strings.HasSuffix(name, ".js")
				hasMap = hasMap || strings.HasSuffix(name, ".map")
			}
			if !hasFont || !hasChunk || hasMap == minify {
				t.Fatalf("unexpected cold outputs: font=%v chunk=%v sourcemap=%v minify=%v", hasFont, hasChunk, hasMap, minify)
			}
			for _, url := range coldMap {
				if !strings.HasPrefix(url, "/static/") {
					t.Fatalf("asset map must retain public URLs: %q", url)
				}
			}

			var mu sync.Mutex
			written := make(map[string]bool)
			var processed atomic.Int64
			opts.OnWrite = func(name string) {
				mu.Lock()
				defer mu.Unlock()
				written[name] = true
			}
			opts.OnAssetProcessed = func() { processed.Add(1) }
			opts.Sched = assetCacheHitScheduler{}
			opts.Sink = NewDiskSink(filepath.Join(root, "warm"), filepath.Dir(opts.DestDir))
			warmMap, err := BuildAssetsEsbuild(opts)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(coldMap, warmMap) {
				t.Fatalf("asset map changed on restore: cold=%v warm=%v", coldMap, warmMap)
			}
			warmFiles := assetTestSnapshot(t, filepath.Join(root, "warm", "static"))
			if !maps.EqualFunc(coldFiles, warmFiles, bytes.Equal) {
				t.Fatalf("warm outputs differ: cold=%v warm=%v", slices.Sorted(maps.Keys(coldFiles)), slices.Sorted(maps.Keys(warmFiles)))
			}
			if len(written) != len(coldFiles) || processed.Load() != int64(len(coldFiles)) {
				t.Fatalf("callbacks missed outputs: writes=%d processed=%d outputs=%d", len(written), processed.Load(), len(coldFiles))
			}
			for name := range coldFiles {
				if !written[filepath.Join(opts.DestDir, filepath.FromSlash(name))] {
					t.Errorf("missing write callback for %s", name)
				}
			}
			if !maps.EqualFunc(sourceBefore, assetTestSnapshot(t, opts.SrcDir), bytes.Equal) {
				t.Fatal("asset builds mutated source files")
			}
			if len(assetTestSnapshot(t, filepath.Dir(opts.DestDir))) != 0 {
				t.Fatal("writes bypassed the staging sink")
			}
		})
	}
}

func TestAssetCacheLegacyAndIncompleteCachesRebuild(t *testing.T) {
	for _, missing := range []string{"outputs.json", "font", "font-symlink", "manifest-symlink", "mode"} {
		t.Run(missing, func(t *testing.T) {
			opts, root := assetCacheFixture(t, true)
			sourceBefore := assetTestSnapshot(t, opts.SrcDir)
			wantMap, err := BuildAssetsEsbuild(opts)
			if err != nil {
				t.Fatal(err)
			}
			wantFiles := assetTestSnapshot(t, filepath.Join(root, "cold", "static"))
			scan, err := scanAssets(opts.SrcFs, opts.SrcDir)
			if err != nil {
				t.Fatal(err)
			}
			cachePath := filepath.Join(opts.CacheDir, scan.hash)
			remove := filepath.Join(cachePath, "outputs.json")
			if missing == "font" || missing == "font-symlink" {
				for name := range wantFiles {
					if strings.HasSuffix(name, ".woff2") {
						remove = filepath.Join(cachePath, filepath.FromSlash(name))
					}
				}
			}
			if missing == "mode" {
				paths := slices.Sorted(maps.Keys(wantFiles))
				saveAssetCache(cachePath, wantMap, paths, false)
			} else {
				if err := os.Remove(remove); err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(missing, "-symlink") {
					if err := os.Symlink(filepath.Join(opts.SrcDir, "css", "main.css"), remove); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				}
			}
			opts.Sink = NewDiskSink(filepath.Join(root, "rebuilt"), filepath.Dir(opts.DestDir))
			gotMap, err := BuildAssetsEsbuild(opts)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(wantMap, gotMap) || !maps.EqualFunc(wantFiles, assetTestSnapshot(t, filepath.Join(root, "rebuilt", "static")), bytes.Equal) {
				t.Fatal("cache miss did not rebuild complete assets")
			}
			if !strings.HasSuffix(missing, "-symlink") {
				opts.Sink = NewDiskSink(filepath.Join(root, "restored"), filepath.Dir(opts.DestDir))
				opts.Sched = assetCacheHitScheduler{}
				if _, err := BuildAssetsEsbuild(opts); err != nil {
					t.Fatalf("rebuilt cache cannot be restored: %v", err)
				}
			}
			if !maps.EqualFunc(sourceBefore, assetTestSnapshot(t, opts.SrcDir), bytes.Equal) {
				t.Fatal("cache fallback mutated source files")
			}
		})
	}
}

func TestAssetCacheRejectsInvalidManifestsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		assetMap string
	}{
		{"malformed", `{`, `{"/static/css/main.css":"/static/css/main.css"}`},
		{"old-version", `{"version":0,"outputs":["css/main.css"]}`, `{}`},
		{"wrong-mode", `{"version":1,"minify":true,"outputs":["css/main.css"]}`, `{}`},
		{"missing-outputs", `{"version":1}`, `{}`},
		{"null-map", `{"version":1,"outputs":["css/main.css"]}`, `null`},
		{"malformed-map", `{"version":1,"outputs":["css/main.css"]}`, `{`},
		{"missing-entrypoint", `{"version":1,"outputs":[]}`, `{"/static/css/main.css":"/static/css/main.css"}`},
		{"url-traversal", `{"version":1,"outputs":["css/main.css"]}`, `{"/static/css/main.css":"/static/../../source/main.css"}`},
		{"url-not-static", `{"version":1,"outputs":["css/main.css"]}`, `{"/static/css/main.css":"/source/main.css"}`},
		{"parent-traversal", `{"version":1,"outputs":["css/main.css","../source/main.css"]}`, `{}`},
		{"embedded-traversal", `{"version":1,"outputs":["css/../../source/main.css"]}`, `{}`},
		{"absolute", `{"version":1,"outputs":["/source/main.css"]}`, `{}`},
		{"windows-drive", `{"version":1,"outputs":["C:/source/main.css"]}`, `{}`},
		{"backslash", `{"version":1,"outputs":["..\\source\\main.css"]}`, `{}`},
		{"empty", `{"version":1,"outputs":[""]}`, `{}`},
		{"dot", `{"version":1,"outputs":["."]}`, `{}`},
		{"nul", `{"version":1,"outputs":["css/\u0000main.css"]}`, `{}`},
		{"empty-secondary", `{"version":1,"outputs":["css/main.css","assets/empty.woff2"]}`, `{}`},
		{"missing-secondary", `{"version":1,"outputs":["css/main.css","assets/missing.woff2"]}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cachePath := filepath.Join(root, "cache")
			writeAssetTestFile(t, filepath.Join(cachePath, "css", "main.css"), []byte("body{}"))
			writeAssetTestFile(t, filepath.Join(cachePath, "assets", "empty.woff2"), nil)
			writeAssetTestFile(t, filepath.Join(cachePath, "outputs.json"), []byte(tc.manifest))
			writeAssetTestFile(t, filepath.Join(cachePath, "map.json"), []byte(tc.assetMap))
			writeAssetTestFile(t, filepath.Join(root, "source", "main.css"), []byte("source sentinel"))
			opts := restoreAssetsOptions{
				cachePath: cachePath, destDir: filepath.Join(root, "public", "static"),
				sink:             NewDiskSink(filepath.Join(root, "stage"), filepath.Join(root, "public")),
				onWrite:          func(string) { t.Error("invalid cache invoked write callback") },
				onAssetProcessed: func() { t.Error("invalid cache invoked progress callback") },
			}
			assets, ok, err := restoreAssetsFromCache(opts)
			if err != nil || ok || assets != nil {
				t.Fatalf("invalid cache must be a miss: assets=%v ok=%v err=%v", assets, ok, err)
			}
			if len(assetTestSnapshot(t, filepath.Join(root, "stage"))) != 0 {
				t.Fatal("invalid cache wrote partial output")
			}
			if got := assetTestSnapshot(t, filepath.Join(root, "source")); string(got["main.css"]) != "source sentinel" {
				t.Fatal("invalid cache mutated source")
			}
		})
	}
}

func TestAssetCacheRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	cachePath := filepath.Join(root, "cache")
	writeAssetTestFile(t, filepath.Join(root, "source", "font.woff2"), []byte("source font"))
	saveAssetCache(cachePath, map[string]string{}, []string{"font.woff2"}, false)
	if err := os.Symlink(filepath.Join(root, "source", "font.woff2"), filepath.Join(cachePath, "font.woff2")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, ok, err := restoreAssetsFromCache(restoreAssetsOptions{
		cachePath: cachePath, destDir: filepath.Join(root, "public", "static"),
		sink: NewDiskSink(filepath.Join(root, "stage"), filepath.Join(root, "public")),
	})
	if err != nil || ok {
		t.Fatalf("escaping cache symlink must be a miss: ok=%v err=%v", ok, err)
	}
}

func TestWriteEsbuildOutputsRejectsSourcePath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "main.css")
	writeAssetTestFile(t, source, []byte("source sentinel"))
	ctx := buildAssetsContext{
		destDir: filepath.Join(root, "public", "static"),
		sink:    NewDiskSink(filepath.Join(root, "stage"), filepath.Join(root, "public")),
	}
	err := writeEsbuildOutputs(ctx, []api.OutputFile{{Path: source, Contents: []byte("bad")}}, filepath.Join(root, "cache"))
	if err == nil {
		t.Fatal("expected output outside destination to fail")
	}
	if got := assetTestSnapshot(t, filepath.Dir(source)); string(got["main.css"]) != "source sentinel" {
		t.Fatal("esbuild output mutated source")
	}
}
