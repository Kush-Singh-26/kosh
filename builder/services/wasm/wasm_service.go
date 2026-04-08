package wasm

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"
)

// Dependencies holds all dependencies for WasmService.
type Dependencies struct {
	Ctx    *buildCtx.BuildContext
	Cfg    *config.Config
	Logger *slog.Logger
	Fs     afero.Fs
}

type wasmService struct {
	ctx    *buildCtx.BuildContext
	cfg    *config.Config
	logger *slog.Logger
	fs     afero.Fs

	searchSourceDirty atomic.Bool
}

// NewService creates a new WasmService with the given dependencies.
func NewService(deps Dependencies) Service {
	return &wasmService{
		ctx:    deps.Ctx,
		cfg:    deps.Cfg,
		logger: deps.Logger,
		fs:     deps.Fs,
	}
}

// CheckAndUpdate recompiles WASM if sources are newer or marked dirty.
func (s *wasmService) CheckAndUpdate(ctx context.Context) (bool, error) {
	// Skip WASM operations in test mode
	if s.ctx != nil && s.ctx.IsTesting {
		return false, nil
	}

	wasmBinary := filepath.Join(s.cfg.KoshSourceRoot, "static", "wasm", "search.wasm")
	if srcMod, err := s.latestSearchSourceModTime(); err == nil {
		if s.searchSourceDirty.Load() {
			if err := assets.CompileWASMFromSource(ctx, fspkg.NormalizePath(filepath.Join(s.cfg.KoshSourceRoot, "cmd", "search", "main.go")), wasmBinary, s.cfg.KoshSourceRoot); err != nil {
				s.logger.Warn("Failed to compile Search WASM", "error", err)
				return false, err
			}
			s.searchSourceDirty.Store(false)
			return true, nil
		} else {
			wasmInfo, statErr := os.Stat(wasmBinary)
			if statErr != nil || srcMod.After(wasmInfo.ModTime()) {
				if err := assets.CompileWASMFromSource(ctx, fspkg.NormalizePath(filepath.Join(s.cfg.KoshSourceRoot, "cmd", "search", "main.go")), wasmBinary, s.cfg.KoshSourceRoot); err != nil {
					s.logger.Warn("Failed to compile Search WASM", "error", err)
					return false, err
				}
				return true, nil
			}
		}
	}

	return false, nil
}

// Deploy ensures the search WASM is available in the output sink.
func (s *wasmService) Deploy(ctx context.Context, sink fspkg.ArtifactSink) error {
	// Skip WASM operations in test mode
	if s.ctx != nil && s.ctx.IsTesting {
		return nil
	}

	wasmBinary := filepath.Join(s.cfg.KoshSourceRoot, "static", "wasm", "search.wasm")
	_, err := s.fs.Stat(wasmBinary)
	sourceAvailable := err == nil

	// Always ensure embedded WASM is deployed if missing or old.
	// Prefer the locally compiled WASM if available so browser/runtime schema
	// always matches the current search.bin generator.
	if sourceAvailable {
		// Use the source WASM (either just rebuilt or already present)
		assets.DeployWASMFromFile(s.fs, sink, s.cfg.CacheDir, wasmBinary)
	} else {
		// No source available (standard user), use embedded WASM
		assets.CheckWASM(sink, s.cfg.CacheDir)
	}

	return nil
}

// SetSearchSourceDirty marks the search source as dirty to force rebuild.
func (s *wasmService) SetSearchSourceDirty(dirty bool) {
	s.searchSourceDirty.Store(dirty)
}

func (s *wasmService) latestSearchSourceModTime() (time.Time, error) {
	paths := []string{
		fspkg.NormalizePath(filepath.Join(s.cfg.KoshSourceRoot, "cmd", "search")),
		fspkg.NormalizePath(filepath.Join(s.cfg.KoshSourceRoot, "builder", "search")),
		fspkg.NormalizePath(filepath.Join(s.cfg.KoshSourceRoot, "builder", "models")),
	}

	latest := time.Time{}
	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		})
		if err != nil {
			return time.Time{}, err
		}
	}

	if latest.IsZero() {
		return time.Time{}, os.ErrNotExist
	}
	return latest, nil
}
