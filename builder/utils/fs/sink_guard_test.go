package fs

import (

	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskSink_WriteFileRejectsOutsideOutputRoots(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, "public.tmp")
	realOut := filepath.Join(base, "public")
	source := filepath.Join(base, "source")

	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	if err := os.MkdirAll(realOut, 0755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	sink := NewDiskSink(staging, realOut)

	sourcePath := filepath.Join(source, "static", "js", "wasm_engine.js")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("failed to create source subdir: %v", err)
	}

	err := sink.WriteFile(sourcePath, []byte("bad"))
	if err == nil {
		t.Fatal("expected write outside output roots to fail")
	}
}

func TestDiskSink_WriteFileMapsOutputToStaging(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, "public.tmp")
	realOut := filepath.Join(base, "public")

	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}

	sink := NewDiskSink(staging, realOut)
	outPath := filepath.Join(realOut, "static", "ok.txt")

	if err := sink.WriteFile(outPath, []byte("hello")); err != nil {
		t.Fatalf("write to output path should succeed: %v", err)
	}

	stagedPath := filepath.Join(staging, "static", "ok.txt")
	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("expected file in staging path %s: %v", stagedPath, err)
	}
}

func TestDiskSink_SetMtimeRejectsOutsideOutputRoots(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, "public.tmp")
	realOut := filepath.Join(base, "public")
	outside := filepath.Join(base, "source", "x.txt")

	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	sink := NewDiskSink(staging, realOut)

	err := sink.SetMtime(outside, time.Now())
	if err == nil {
		t.Fatal("expected SetMtime outside output roots to fail")
	}
}
