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
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"
)

// Dependencies holds all dependencies for WasmService.
type Dependencies struct {
	Ctx    *buildctx.BuildContext
	Cfg    *config.Config
	Logger *slog.Logger
	SourceFs afero.Fs
}

type wasmService struct {
	ctx    *buildctx.BuildContext
	cfg    *config.Config
	logger *slog.Logger
	sourceFs afero.Fs

	searchSourceDirty atomic.Bool
}

// NewService creates a new WasmService with the given dependencies.
func NewService(dependencies Dependencies) Service {
	return &wasmService{
		ctx:    dependencies.Ctx,
		cfg:    dependencies.Cfg,
		logger: dependencies.Logger,
		sourceFs: dependencies.SourceFs,
	}
}

// CheckAndUpdate recompiles WASM if sources are newer or marked dirty.
func (service *wasmService) CheckAndUpdate(ctx context.Context) (bool, error) {
	// Skip WASM operations in test mode
	if service.ctx != nil && service.ctx.IsTesting {
		return false, nil
	}

	wasmBinaryPath := filepath.Join(service.cfg.KoshSourceRoot, "static", "wasm", "search.wasm")
	if sourceModificationTime, err := service.latestSearchSourceModTime(); err == nil {
		if service.searchSourceDirty.Load() {
			if err := assets.CompileWASMFromSource(ctx, fspkg.NormalizePath(filepath.Join(service.cfg.KoshSourceRoot, "cmd", "search", "main.go")), wasmBinaryPath, service.cfg.KoshSourceRoot); err != nil {
				service.logger.Warn("Failed to compile Search WASM", "error", err)
				return false, err
			}
			service.searchSourceDirty.Store(false)
			return true, nil
		} else {
			wasmFileInformation, statError := os.Stat(wasmBinaryPath)
			if statError != nil || sourceModificationTime.After(wasmFileInformation.ModTime()) {
				if err := assets.CompileWASMFromSource(ctx, fspkg.NormalizePath(filepath.Join(service.cfg.KoshSourceRoot, "cmd", "search", "main.go")), wasmBinaryPath, service.cfg.KoshSourceRoot); err != nil {
					service.logger.Warn("Failed to compile Search WASM", "error", err)
					return false, err
				}
				return true, nil
			}
		}
	}

	return false, nil
}

// Deploy ensures the search WASM is available in the output sink.
func (service *wasmService) Deploy(ctx context.Context, sink fspkg.ArtifactSink) error {
	// Skip WASM operations in test mode
	if service.ctx != nil && service.ctx.IsTesting {
		return nil
	}

	wasmBinaryPath := filepath.Join(service.cfg.KoshSourceRoot, "static", "wasm", "search.wasm")
	_, err := service.sourceFs.Stat(wasmBinaryPath)
	sourceAvailable := err == nil

	// Always ensure embedded WASM is deployed if missing or old.
	// Prefer the locally compiled WASM if available so browser/runtime schema
	// always matches the current search.bin generator.
	if sourceAvailable {
		// Use the source WASM (either just rebuilt or already present)
		assets.DeployWASMFromFile(service.sourceFs, sink, service.cfg.CacheDir, wasmBinaryPath)
	} else {
		// No source available (standard user), use embedded WASM
		assets.CheckWASM(sink, service.cfg.CacheDir)
	}

	return nil
}

// SetSearchSourceDirty marks the search source as dirty to force rebuild.
func (service *wasmService) SetSearchSourceDirty(dirty bool) {
	service.searchSourceDirty.Store(dirty)
}

func (service *wasmService) latestSearchSourceModTime() (time.Time, error) {
	searchPaths := []string{
		fspkg.NormalizePath(filepath.Join(service.cfg.KoshSourceRoot, "cmd", "search")),
		fspkg.NormalizePath(filepath.Join(service.cfg.KoshSourceRoot, "builder", "search")),
		fspkg.NormalizePath(filepath.Join(service.cfg.KoshSourceRoot, "builder", "models")),
	}

	latestModificationTime := time.Time{}
	for _, searchPath := range searchPaths {
		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			if info.ModTime().After(latestModificationTime) {
				latestModificationTime = info.ModTime()
			}
			return nil
		})
		if err != nil {
			return time.Time{}, err
		}
	}

	if latestModificationTime.IsZero() {
		return time.Time{}, os.ErrNotExist
	}
	return latestModificationTime, nil
}
