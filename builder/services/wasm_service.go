package services

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	"github.com/spf13/afero"
)

// WasmServiceDependencies holds all dependencies for WasmService.
type WasmServiceDependencies struct {
	Cfg    *config.Config
	Logger *slog.Logger
	Fs     afero.Fs
}

type wasmService struct {
	cfg    *config.Config
	logger *slog.Logger
	fs     afero.Fs

	searchSourceDirty atomic.Bool
}

// NewWasmService creates a new WasmService with the given dependencies.
func NewWasmService(deps WasmServiceDependencies) WasmService {
	return &wasmService{
		cfg:    deps.Cfg,
		logger: deps.Logger,
		fs:     deps.Fs,
	}
}

func (s *wasmService) CheckAndUpdate(ctx context.Context) error {
	// Skip WASM operations in test mode
	if utils.TestingMode {
		return nil
	}

	wasmBinary := fspkg.RepoPath("static", "wasm", "search.wasm")
	if srcMod, err := s.latestSearchSourceModTime(); err == nil {
		if s.searchSourceDirty.Load() {
			if err := assets.CompileWASMFromSource(ctx, fspkg.RepoPath("cmd", "search", "main.go"), wasmBinary); err != nil {
				s.logger.Warn("Failed to compile Search WASM", "error", err)
				return err
			}
			s.searchSourceDirty.Store(false)
		} else {
			wasmInfo, statErr := os.Stat(wasmBinary)
			if statErr != nil || srcMod.After(wasmInfo.ModTime()) {
				if err := assets.CompileWASMFromSource(ctx, fspkg.RepoPath("cmd", "search", "main.go"), wasmBinary); err != nil {
					s.logger.Warn("Failed to compile Search WASM", "error", err)
					return err
				}
			}
		}
	}

	return nil
}

func (s *wasmService) Deploy(ctx context.Context, stagingDir string) error {
	// Skip WASM operations in test mode
	if utils.TestingMode {
		return nil
	}

	wasmBinary := fspkg.RepoPath("static", "wasm", "search.wasm")
	_, err := s.fs.Stat(wasmBinary)
	sourceAvailable := err == nil

	// Always ensure embedded WASM is deployed if missing or old.
	// Prefer the locally compiled WASM if available so browser/runtime schema
	// always matches the current search.bin generator.
	if sourceAvailable {
		// Use the source WASM (either just rebuilt or already present)
		assets.DeployWASMFromFile(s.fs, stagingDir, s.cfg.CacheDir, wasmBinary)
	} else {
		// No source available (standard user), use embedded WASM
		assets.CheckWASM(stagingDir, s.cfg.CacheDir)
	}

	return nil
}

func (s *wasmService) SetSearchSourceDirty(dirty bool) {
	s.searchSourceDirty.Store(dirty)
}

func (s *wasmService) latestSearchSourceModTime() (time.Time, error) {
	paths := []string{
		fspkg.RepoPath("cmd", "search"),
		fspkg.RepoPath("builder", "search"),
		fspkg.RepoPath("builder", "models"),
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
